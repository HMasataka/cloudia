//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/admin"
	"github.com/HMasataka/cloudia/internal/auth"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway"
	"github.com/HMasataka/cloudia/internal/logging"
	"github.com/HMasataka/cloudia/internal/protocol"
	"github.com/HMasataka/cloudia/internal/service"
)

// randomPort は利用可能なランダムポートを返します。
func randomPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestIntegration_GatewayServer(t *testing.T) {
	// a. 設定読み込み（ポートをランダムに変更）
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	port := randomPort(t)
	cfg.Server.Port = port
	cfg.Server.Host = "127.0.0.1"

	// b. ロガー初期化
	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		t.Fatalf("logging.NewLogger failed: %v", err)
	}
	defer logger.Sync()

	// c. サーバー構築
	verifiers := map[string]auth.Verifier{}
	codecs := map[string]protocol.Codec{}
	registry := service.NewRegistry()
	adminHandler := admin.NewHandler(nil, nil, registry, cfg, logger)
	serviceHandler := gateway.NewServiceHandler(verifiers, codecs, registry, logger)
	router := gateway.NewRouter(context.Background(), adminHandler, serviceHandler, logger, cfg.Server.Timeout)
	server := gateway.NewServer(cfg.Server, cfg.Endpoints, cfg.Metrics, router, logger)

	// d. サーバー起動
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start failed: %v", err)
	}

	// サーバーが起動するまで待機
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForServer(baseURL+"/health", 3*time.Second); err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}

	// e. GET /health -> 200 + {"status":"ok"}
	t.Run("health endpoint returns 200 with status ok", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if result["status"] != "ok" {
			t.Errorf("expected status=ok, got %q", result["status"])
		}
	})

	// f. GET /admin/services -> 200 + {"services":[]}
	t.Run("services endpoint returns 200 with empty services", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/admin/services")
		if err != nil {
			t.Fatalf("GET /admin/services failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		var result map[string][]string
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		services, ok := result["services"]
		if !ok {
			t.Fatalf("expected 'services' key in response")
		}
		if len(services) != 0 {
			t.Errorf("expected empty services, got %v", services)
		}
	})

	// g. 未対応パスへのリクエストで 400 が返ること
	t.Run("unknown path returns 400", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/unknown/path")
		if err != nil {
			t.Fatalf("GET /unknown/path failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
	})

	// h. server.Shutdown で正常終了
	t.Run("graceful shutdown", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("server.Shutdown failed: %v", err)
		}
	})
}

// waitForServer は指定 URL が 200 を返すまでポーリングします。
func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server at %s did not become ready within %s", url, timeout)
}
