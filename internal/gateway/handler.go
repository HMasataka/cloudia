package gateway

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/HMasataka/cloudia/internal/auth"
	"github.com/HMasataka/cloudia/internal/protocol"
	awsprotocol "github.com/HMasataka/cloudia/internal/protocol/aws"
	gcpprotocol "github.com/HMasataka/cloudia/internal/protocol/gcp"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

// ServiceHandler はプロバイダ検出・認証・プロトコル変換・サービス解決・ディスパッチ・
// レスポンスエンコードを一貫して行う HTTP ハンドラです。
type ServiceHandler struct {
	verifiers map[string]auth.Verifier
	codecs    map[string]protocol.Codec
	registry  *service.Registry
	logger    *zap.Logger
}

// NewServiceHandler は ServiceHandler を生成します。
func NewServiceHandler(
	verifiers map[string]auth.Verifier,
	codecs map[string]protocol.Codec,
	registry *service.Registry,
	logger *zap.Logger,
) *ServiceHandler {
	return &ServiceHandler{
		verifiers: verifiers,
		codecs:    codecs,
		registry:  registry,
		logger:    logger,
	}
}

// ServeHTTP はリクエストを処理します。
// バーチャルホスト書き換え -> プロバイダ検出 -> 認証 -> プロトコル変換 -> サービス解決 -> ディスパッチ -> レスポンスエンコード
func (h *ServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rewriteVirtualHostPath(r)

	requestID, err := GenerateRequestID()
	if err != nil {
		http.Error(w, "failed to generate request id", http.StatusInternalServerError)
		return
	}

	ctx := WithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)

	h.logger.Info("incoming request",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("request_id", requestID),
	)

	provider, err := auth.DetectProvider(r)
	if err != nil {
		http.Error(w, "400 Bad Request: cannot detect cloud provider", http.StatusBadRequest)
		return
	}

	codec, ok := h.codecs[provider]
	if !ok {
		http.Error(w, fmt.Sprintf("400 Bad Request: unsupported provider %q", provider), http.StatusBadRequest)
		return
	}

	verifier, ok := h.verifiers[provider]
	if !ok {
		codec.EncodeError(w, fmt.Errorf("unsupported provider %q", provider), requestID)
		return
	}

	authResult, err := verifier.Verify(r)
	if err != nil {
		h.logger.Warn("authentication failed",
			zap.String("provider", provider),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		h.encodeAuthError(w, provider, err, requestID)
		return
	}

	if authResult.Service != "" {
		svc, err := h.registry.Resolve(provider, authResult.Service)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				h.encodeNotImplemented(w, provider, authResult.Service, requestID)
				return
			}
			codec.EncodeError(w, err, requestID)
			return
		}
		if proxySvc, ok := svc.(service.ProxyService); ok {
			w.Header().Set("X-Request-Id", requestID)
			proxySvc.ServeHTTP(w, r)
			return
		}
	}

	req, err := codec.DecodeRequest(r)
	if err != nil {
		codec.EncodeError(w, err, requestID)
		return
	}

	if authResult.Service != "" {
		req.Service = authResult.Service
	}

	svc, err := h.registry.Resolve(provider, req.Service)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			h.encodeNotImplemented(w, provider, req.Service, requestID)
			return
		}
		codec.EncodeError(w, err, requestID)
		return
	}

	if proxySvc, ok := svc.(service.ProxyService); ok {
		w.Header().Set("X-Request-Id", requestID)
		proxySvc.ServeHTTP(w, r)
		return
	}

	resp, err := svc.HandleRequest(ctx, req)
	if err != nil {
		codec.EncodeError(w, err, requestID)
		return
	}

	if resp.Headers == nil {
		resp.Headers = map[string]string{}
	}
	resp.Headers["X-Request-Id"] = requestID

	codec.EncodeResponse(w, resp)
}

// encodeAuthError は認証エラーをプロバイダ別フォーマットで返します。
func (h *ServiceHandler) encodeAuthError(w http.ResponseWriter, provider string, _ error, requestID string) {
	switch provider {
	case "aws":
		awsprotocol.WriteError(w, http.StatusForbidden, "SignatureDoesNotMatch", "the request signature we calculated does not match the signature you provided", requestID)
	case "gcp":
		gcpprotocol.WriteError(w, http.StatusUnauthorized, "request had invalid authentication credentials")
	default:
		http.Error(w, "authentication failed", http.StatusUnauthorized)
	}
}

// encodeNotImplemented は未登録サービスへの 501 エラーをプロバイダ別フォーマットで返します。
func (h *ServiceHandler) encodeNotImplemented(w http.ResponseWriter, provider, svcName string, requestID string) {
	msg := fmt.Sprintf("service %q is not supported", svcName)

	switch provider {
	case "aws":
		awsprotocol.WriteError(w, http.StatusNotImplemented, "UnsupportedOperation", msg, requestID)
	case "gcp":
		gcpprotocol.WriteError(w, http.StatusNotImplemented, msg)
	default:
		http.Error(w, msg, http.StatusNotImplemented)
	}
}
