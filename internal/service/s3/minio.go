package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
)

const (
	minioImage         = "minio/minio:latest"
	minioContainerName = "cloudia-minio"
	minioContainerPort = "9000"
	minioServiceLabel  = "s3"

	healthCheckInterval = time.Second
	healthCheckMaxTries = 30
)

// minioBackend manages the lifecycle and proxying of a MinIO container.
type minioBackend struct {
	containerID string
	hostPort    int
	baseURL     string // "http://host:port"
	runner      service.ContainerRunner
	portAlloc   service.PortAllocator
}

// newMinioBackendWithURL creates a minioBackend pointing to an existing endpoint.
// Used for testing without Docker integration.
func newMinioBackendWithURL(endpoint string) *minioBackend {
	return &minioBackend{baseURL: endpoint}
}

// Init starts or reuses a MinIO container and waits for it to be ready.
func (m *minioBackend) Init(ctx context.Context, cfg config.AWSAuthConfig, deps service.ServiceDeps) error {
	hostPort, err := deps.PortAllocator.Allocate(9000, "minio")
	if err != nil {
		return fmt.Errorf("s3: allocate port: %w", err)
	}
	m.hostPort = hostPort
	m.baseURL = fmt.Sprintf("http://localhost:%d", hostPort)
	m.runner = deps.DockerClient
	m.portAlloc = deps.PortAllocator

	containerID, err := m.findOrCreateContainer(ctx, cfg, deps, hostPort)
	if err != nil {
		deps.PortAllocator.Release(hostPort)
		return err
	}

	m.containerID = containerID

	if err := m.waitHealthy(ctx); err != nil {
		return fmt.Errorf("s3: minio health check failed: %w", err)
	}

	return nil
}

// findOrCreateContainer reuses an existing MinIO container or creates a new one.
func (m *minioBackend) findOrCreateContainer(
	ctx context.Context,
	cfg config.AWSAuthConfig,
	deps service.ServiceDeps,
	hostPort int,
) (string, error) {
	if finder, ok := deps.DockerClient.(*docker.Client); ok {
		existing, err := finder.FindContainerByServiceLabel(ctx, minioServiceLabel)
		if err != nil {
			return "", fmt.Errorf("s3: find existing container: %w", err)
		}
		if existing != nil {
			return existing.ID, nil
		}
	}

	return m.runner.RunContainer(ctx, docker.ContainerConfig{
		Image: minioImage,
		Name:  minioContainerName,
		Labels: map[string]string{
			docker.LabelService: minioServiceLabel,
		},
		Env: map[string]string{
			"MINIO_ROOT_USER":     cfg.AccessKey,
			"MINIO_ROOT_PASSWORD": cfg.SecretKey,
		},
		Ports: map[string]string{
			minioContainerPort: strconv.Itoa(hostPort),
		},
		Cmd: []string{"server", "/data"},
	})
}

// Shutdown stops and removes the MinIO container, then releases the allocated port.
func (m *minioBackend) Shutdown(ctx context.Context) error {
	if m.containerID == "" {
		return nil
	}

	if err := m.runner.StopContainer(ctx, m.containerID, nil); err != nil {
		return fmt.Errorf("s3: stop container: %w", err)
	}

	if err := m.runner.RemoveContainer(ctx, m.containerID); err != nil {
		return fmt.Errorf("s3: remove container: %w", err)
	}

	m.portAlloc.Release(m.hostPort)
	m.containerID = ""

	return nil
}

// Health checks whether MinIO is ready by calling its health endpoint.
func (m *minioBackend) Health(ctx context.Context) service.HealthStatus {
	if err := m.callHealthEndpoint(ctx); err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// ServeHTTP reverse-proxies the request to the MinIO container.
func (m *minioBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(m.baseURL)
	if err != nil {
		http.Error(w, "invalid minio endpoint", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

// waitHealthy polls the MinIO health endpoint until it responds or the attempt limit is reached.
func (m *minioBackend) waitHealthy(ctx context.Context) error {
	for i := 0; i < healthCheckMaxTries; i++ {
		if err := m.callHealthEndpoint(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("minio did not become ready after %d attempts", healthCheckMaxTries)
}

// callHealthEndpoint performs a single HTTP GET to the MinIO health ready endpoint.
func (m *minioBackend) callHealthEndpoint(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/minio/health/ready", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
