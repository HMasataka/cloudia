package imds_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/gateway/imds"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const testClientIP = "10.0.1.5"

// newTestServer はテスト用IMDSサーバーを起動し、ベースURLとクリーンアップ関数を返します。
func newTestServer(t *testing.T, store state.Store) string {
	t.Helper()
	logger := zap.NewNop()
	srv := imds.New("127.0.0.1:0", store, logger)
	if err := srv.Start(); err != nil {
		t.Fatalf("imds server start failed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return "http://" + srv.Addr()
}

// newStoreWithInstance はEC2インスタンスを持つメモリストアを生成します。
func newStoreWithInstance(t *testing.T) state.Store {
	t.Helper()
	store := state.NewMemoryStore()
	r := &models.Resource{
		Kind:     "aws:ec2:instance",
		ID:       "i-0123456789abcdef0",
		Provider: "aws",
		Service:  "ec2",
		Region:   "ap-northeast-1",
		Spec: map[string]interface{}{
			"instanceId":       "i-0123456789abcdef0",
			"imageId":          "ami-0abcdef1234567890",
			"instanceType":     "t3.micro",
			"privateIpAddress": testClientIP,
			"privateDnsName":   "ip-10-0-1-5.ap-northeast-1.compute.internal",
		},
		Status: "active",
	}
	if err := store.Put(context.Background(), r); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}
	return store
}

// doRequest はクライアントIPをX-Forwarded-Forで偽装してリクエストを送ります。
func doRequest(t *testing.T, method, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	// クライアントIPをX-Forwarded-Forで偽装
	req.Header.Set("X-Forwarded-For", testClientIP)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request failed: %v", err)
	}
	return resp
}

// getToken はIMDSv2トークンを取得します。
func getToken(t *testing.T, baseURL string, ttlSec int) string {
	t.Helper()
	resp := doRequest(t, http.MethodPut, baseURL+"/latest/api/token", map[string]string{
		"X-aws-ec2-metadata-token-ttl-seconds": fmt.Sprintf("%d", ttlSec),
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	return string(b)
}

func TestIMDSv2_TokenGet(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	token := getToken(t, baseURL, 300)
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestIMDSv2_MetadataWithToken(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	token := getToken(t, baseURL, 300)

	resp := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/instance-id", map[string]string{
		"X-aws-ec2-metadata-token": token,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(b) != "i-0123456789abcdef0" {
		t.Errorf("expected instance-id %q, got %q", "i-0123456789abcdef0", string(b))
	}
}

func TestIMDSv1_MetadataWithoutToken(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	// IMDSv1: トークンなし
	resp := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/instance-id", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for IMDSv1 compatibility, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(b) != "i-0123456789abcdef0" {
		t.Errorf("expected instance-id %q, got %q", "i-0123456789abcdef0", string(b))
	}
}

func TestIMDS_AllMetadataPaths(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	token := getToken(t, baseURL, 300)
	headers := map[string]string{"X-aws-ec2-metadata-token": token}

	tests := []struct {
		path     string
		expected string
	}{
		{"/latest/meta-data/instance-id", "i-0123456789abcdef0"},
		{"/latest/meta-data/ami-id", "ami-0abcdef1234567890"},
		{"/latest/meta-data/instance-type", "t3.micro"},
		{"/latest/meta-data/local-ipv4", testClientIP},
		{"/latest/meta-data/placement/availability-zone", "ap-northeast-1a"},
		{"/latest/meta-data/hostname", "ip-10-0-1-5.ap-northeast-1.compute.internal"},
		{"/latest/meta-data/instance-action", "none"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			resp := doRequest(t, http.MethodGet, baseURL+tc.path, headers)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body failed: %v", err)
			}
			if string(b) != tc.expected {
				t.Errorf("path %s: expected %q, got %q", tc.path, tc.expected, string(b))
			}
		})
	}
}

func TestIMDS_TokenExpired(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	// TTL 1秒のトークンを取得
	token := getToken(t, baseURL, 1)

	// トークンが有効なうちはアクセスできることを確認
	resp := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/instance-id", map[string]string{
		"X-aws-ec2-metadata-token": token,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 before expiry, got %d", resp.StatusCode)
	}

	// トークンの有効期限が切れるまで待機
	time.Sleep(1100 * time.Millisecond)

	resp2 := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/instance-id", map[string]string{
		"X-aws-ec2-metadata-token": token,
	})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after expiry, got %d", resp2.StatusCode)
	}
}

func TestIMDS_NoInstance_Returns404(t *testing.T) {
	// インスタンスのないストアでテスト
	store := state.NewMemoryStore()
	baseURL := newTestServer(t, store)

	resp := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/instance-id", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when no instance found, got %d", resp.StatusCode)
	}
}

func TestIMDS_MetadataList(t *testing.T) {
	store := newStoreWithInstance(t)
	baseURL := newTestServer(t, store)

	resp := doRequest(t, http.MethodGet, baseURL+"/latest/meta-data/", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	body := string(b)
	for _, key := range []string{"instance-id", "ami-id", "instance-type", "local-ipv4", "placement/availability-zone", "hostname", "instance-action"} {
		found := false
		for _, line := range splitLines(body) {
			if line == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected key %q in metadata list, got:\n%s", key, body)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
