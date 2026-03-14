package imds

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	ec2InstanceKind = "aws:ec2:instance"
)

// Handler はIMDSのHTTPハンドラです。
type Handler struct {
	store      state.Store
	tokenStore *TokenStore
	logger     *zap.Logger
}

// NewHandler は Handler を生成します。
func NewHandler(store state.Store, tokenStore *TokenStore, logger *zap.Logger) *Handler {
	return &Handler{
		store:      store,
		tokenStore: tokenStore,
		logger:     logger,
	}
}

// ServeHTTP はIMDSリクエストをルーティングします。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// IMDSv2 トークン生成
	if r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" {
		h.handleTokenPut(w, r)
		return
	}

	// メタデータエンドポイント
	if strings.HasPrefix(r.URL.Path, "/latest/meta-data") {
		h.handleMetaData(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleTokenPut はIMDSv2トークンを生成します。
func (h *Handler) handleTokenPut(w http.ResponseWriter, r *http.Request) {
	ttlStr := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
	if ttlStr == "" {
		http.Error(w, "missing X-aws-ec2-metadata-token-ttl-seconds header", http.StatusBadRequest)
		return
	}

	const maxTTLSec = 21600 // AWS 仕様準拠の最大値（6時間）
	ttlSec, err := strconv.Atoi(ttlStr)
	if err != nil || ttlSec <= 0 {
		http.Error(w, "invalid X-aws-ec2-metadata-token-ttl-seconds header", http.StatusBadRequest)
		return
	}
	if ttlSec > maxTTLSec {
		ttlSec = maxTTLSec
	}

	token, err := h.tokenStore.Generate(time.Duration(ttlSec) * time.Second)
	if err != nil {
		h.logger.Error("imds: failed to generate token", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, token)
}

// handleMetaData はメタデータリクエストを処理します。
func (h *Handler) handleMetaData(w http.ResponseWriter, r *http.Request) {
	// IMDSv2 トークン検証（IMDSv1 互換のためトークンがない場合も許可）
	token := r.Header.Get("X-aws-ec2-metadata-token")
	if token != "" && !h.tokenStore.Validate(token) {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	clientIP := extractClientIP(r)

	instance, err := h.findInstanceByIP(r.Context(), clientIP)
	if err != nil || instance == nil {
		http.NotFound(w, r)
		return
	}

	path := r.URL.Path
	// /latest/meta-data/ のリスト
	if path == "/latest/meta-data/" || path == "/latest/meta-data" {
		keys := []string{
			"instance-id",
			"ami-id",
			"instance-type",
			"local-ipv4",
			"placement/availability-zone",
			"hostname",
			"instance-action",
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Join(keys, "\n"))
		return
	}

	spec := instance.Spec
	var value string

	switch path {
	case "/latest/meta-data/instance-id":
		value = stringFromSpec(spec, "instanceId", instance.ID)
	case "/latest/meta-data/ami-id":
		value = stringFromSpec(spec, "imageId", "")
	case "/latest/meta-data/instance-type":
		value = stringFromSpec(spec, "instanceType", "")
	case "/latest/meta-data/local-ipv4":
		value = stringFromSpec(spec, "privateIpAddress", clientIP)
	case "/latest/meta-data/placement/availability-zone":
		az := stringFromSpec(spec, "availabilityZone", "")
		if az == "" {
			az = instance.Region + "a"
		}
		value = az
	case "/latest/meta-data/hostname":
		value = stringFromSpec(spec, "privateDnsName", "")
	case "/latest/meta-data/instance-action":
		value = "none"
	default:
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, value)
}

// findInstanceByIP はクライアントIPからEC2インスタンスを検索します。
func (h *Handler) findInstanceByIP(ctx context.Context, clientIP string) (*models.Resource, error) {
	resources, err := h.store.List(ctx, ec2InstanceKind, state.Filter{})
	if err != nil {
		return nil, err
	}

	for _, r := range resources {
		ip := stringFromSpec(r.Spec, "privateIpAddress", "")
		if ip == clientIP {
			return r, nil
		}
	}
	return nil, nil
}

// extractClientIP はリクエストからクライアントIPを抽出します。
// RemoteAddr のみを使用し、X-Forwarded-For ヘッダーは信頼しません。
func extractClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// stringFromSpec は Spec マップから文字列を取得します。キーが存在しない場合は fallback を返します。
func stringFromSpec(spec map[string]interface{}, key, fallback string) string {
	if spec == nil {
		return fallback
	}
	v, ok := spec[key]
	if !ok {
		return fallback
	}
	s, ok := v.(string)
	if !ok {
		return fallback
	}
	return s
}
