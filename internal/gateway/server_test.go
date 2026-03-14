package gateway_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/gateway"
)

func freePort(t *testing.T) int {
	t.Helper()
	// net.Listen on :0 lets the OS assign a free port.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("could not get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestServer_StartShutdown_MainPortOnly(t *testing.T) {
	// Given: no service endpoints configured
	port := freePort(t)
	cfg := config.ServerConfig{Host: "127.0.0.1", Port: port}
	endpointsCfg := config.EndpointsConfig{}
	logger := zap.NewNop()

	srv := gateway.NewServer(cfg, endpointsCfg, http.NewServeMux(), logger)

	// When: server starts
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Then: main port is reachable
	assertPortOpen(t, port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestServer_StartShutdown_WithEndpoints(t *testing.T) {
	// Given: one service endpoint configured
	mainPort := freePort(t)
	svcPort := freePort(t)
	cfg := config.ServerConfig{Host: "127.0.0.1", Port: mainPort}
	endpointsCfg := config.EndpointsConfig{
		Services: map[string]config.ServiceEndpointConfig{
			"aws.s3": {Port: svcPort},
		},
	}
	logger := zap.NewNop()

	srv := gateway.NewServer(cfg, endpointsCfg, http.NewServeMux(), logger)

	// When: server starts
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Then: both main port and service port are reachable
	assertPortOpen(t, mainPort)
	assertPortOpen(t, svcPort)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestServer_Shutdown_StopsAllListeners(t *testing.T) {
	// Given: server with one endpoint running
	mainPort := freePort(t)
	svcPort := freePort(t)
	cfg := config.ServerConfig{Host: "127.0.0.1", Port: mainPort}
	endpointsCfg := config.EndpointsConfig{
		Services: map[string]config.ServiceEndpointConfig{
			"aws.s3": {Port: svcPort},
		},
	}
	logger := zap.NewNop()

	srv := gateway.NewServer(cfg, endpointsCfg, http.NewServeMux(), logger)

	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// When: shutdown is called
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Then: ports are no longer accepting connections
	assertPortClosed(t, mainPort)
	assertPortClosed(t, svcPort)
}

func assertPortOpen(t *testing.T, port int) {
	t.Helper()
	// Allow a brief moment for the goroutine to Serve.
	time.Sleep(20 * time.Millisecond)
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Errorf("expected port %d to be open, got: %v", port, err)
		return
	}
	conn.Close()
}

func assertPortClosed(t *testing.T, port int) {
	t.Helper()
	// Allow a brief moment for shutdown to propagate.
	time.Sleep(20 * time.Millisecond)
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		conn.Close()
		t.Errorf("expected port %d to be closed, but connection succeeded", port)
	}
}
