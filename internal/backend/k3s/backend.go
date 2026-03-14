package k3s

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

const (
	k3sImage            = "rancher/k3s:v1.29.0-k3s1"
	k3sContainerPort    = "6443"
	k3sNetwork          = "cloudia"
	healthCheckMaxTries = 60
	healthCheckInterval = time.Second
)

// K3sBackend manages the lifecycle of a k3s Kubernetes cluster container.
type K3sBackend struct {
	runner      service.ContainerRunner
	logger      *zap.Logger
	containerID string
	host        string
	port        string
	kubeconfig  string
	caData      string
	portAlloc   service.PortAllocator
	hostPort    int
}

// NewK3sBackend creates a new K3sBackend with the given logger.
func NewK3sBackend(logger *zap.Logger) *K3sBackend {
	return &K3sBackend{
		logger: logger,
		host:   "localhost",
	}
}

// Start starts a k3s container, waits for it to be healthy, and retrieves its kubeconfig.
func (b *K3sBackend) Start(ctx context.Context, deps service.ServiceDeps, clusterName string) error {
	b.runner = deps.DockerClient
	b.portAlloc = deps.PortAllocator

	hostPort, err := deps.PortAllocator.Allocate(6443, "cloudia-k3s-"+clusterName)
	if err != nil {
		return fmt.Errorf("k3s: allocate port: %w", err)
	}
	b.hostPort = hostPort
	b.port = strconv.Itoa(hostPort)

	containerID, err := b.runner.RunContainer(ctx, docker.ContainerConfig{
		Image:      k3sImage,
		Name:       "cloudia-k3s-" + clusterName,
		Privileged: true,
		Cmd:        []string{"server", "--disable=traefik"},
		Ports: map[string]string{
			k3sContainerPort: b.port,
		},
		Network: k3sNetwork,
	})
	if err != nil {
		deps.PortAllocator.Release(hostPort)
		return fmt.Errorf("k3s: run container: %w", err)
	}
	b.containerID = containerID

	if err := b.waitHealthy(ctx); err != nil {
		return fmt.Errorf("k3s: health check failed: %w", err)
	}

	if err := b.fetchKubeconfig(ctx); err != nil {
		return fmt.Errorf("k3s: fetch kubeconfig: %w", err)
	}

	b.logger.Info("k3s backend ready", zap.String("host", b.host), zap.String("port", b.port))
	return nil
}

// waitHealthy polls the k3s /readyz endpoint until it returns 200 or the attempt limit is reached.
func (b *K3sBackend) waitHealthy(ctx context.Context) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	url := fmt.Sprintf("https://localhost:%s/readyz", b.port)

	for i := 0; i < healthCheckMaxTries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("k3s: create health check request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("k3s: container did not become ready after %d attempts", healthCheckMaxTries)
}

// fetchKubeconfig retrieves and patches the kubeconfig from the running k3s container.
func (b *K3sBackend) fetchKubeconfig(ctx context.Context) error {
	output, err := b.runner.ExecInContainer(ctx, b.containerID, []string{"cat", "/etc/rancher/k3s/k3s.yaml"})
	if err != nil {
		return fmt.Errorf("k3s: exec kubeconfig: %w", err)
	}

	kubeconfig := string(output)

	// Replace the server address with the host-mapped port.
	kubeconfig = strings.ReplaceAll(
		kubeconfig,
		"server: https://127.0.0.1:6443",
		fmt.Sprintf("server: https://127.0.0.1:%s", b.port),
	)

	// Extract CA data from kubeconfig.
	caData, err := extractCAData(kubeconfig)
	if err != nil {
		return fmt.Errorf("k3s: extract CA data: %w", err)
	}

	b.kubeconfig = kubeconfig
	b.caData = caData
	return nil
}

// extractCAData parses the certificate-authority-data field from a kubeconfig string.
func extractCAData(kubeconfig string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(kubeconfig))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "certificate-authority-data:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("certificate-authority-data not found in kubeconfig")
}

// Kubeconfig returns the rewritten kubeconfig with the host-mapped server address.
func (b *K3sBackend) Kubeconfig() string {
	return b.kubeconfig
}

// Endpoint returns the HTTPS endpoint for the k3s API server.
func (b *K3sBackend) Endpoint() string {
	return fmt.Sprintf("https://localhost:%s", b.port)
}

// CertificateAuthority returns the base64-encoded CA data from the kubeconfig.
func (b *K3sBackend) CertificateAuthority() string {
	return b.caData
}

// ContainerID returns the Docker container ID of the k3s container.
func (b *K3sBackend) ContainerID() string {
	return b.containerID
}

// Health checks whether the k3s API server is reachable via /readyz.
func (b *K3sBackend) Health(ctx context.Context) service.HealthStatus {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	url := fmt.Sprintf("https://localhost:%s/readyz", b.port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}

	resp, err := client.Do(req)
	if err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return service.HealthStatus{Healthy: false, Message: fmt.Sprintf("unexpected status: %d", resp.StatusCode)}
	}

	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Shutdown stops and removes the k3s container, then releases the allocated port.
func (b *K3sBackend) Shutdown(ctx context.Context) error {
	if b.containerID == "" {
		return nil
	}

	if err := b.runner.StopContainer(ctx, b.containerID, nil); err != nil {
		return fmt.Errorf("k3s: stop container: %w", err)
	}

	if err := b.runner.RemoveContainer(ctx, b.containerID); err != nil {
		return fmt.Errorf("k3s: remove container: %w", err)
	}

	b.portAlloc.Release(b.hostPort)
	b.containerID = ""

	return nil
}
