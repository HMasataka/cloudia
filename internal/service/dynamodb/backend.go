package dynamodb

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
)

const (
	dynamodbImage         = "amazon/dynamodb-local:2.5.3"
	dynamodbContainerName = "cloudia-dynamodb"
	dynamodbContainerPort = "8000"
	dynamodbServiceLabel  = "dynamodb"

	healthCheckInterval = time.Second
	healthCheckMaxTries = 30
)

// dynamodbBackend manages the lifecycle and proxying of a DynamoDB Local container.
type dynamodbBackend struct {
	containerID string
	hostPort    int
	baseURL     string // "http://host:port"
	runner      service.ContainerRunner
	portAlloc   service.PortAllocator
}

// newDynamoDBBackendWithURL creates a dynamodbBackend pointing to an existing endpoint.
// Used for testing without Docker integration.
func newDynamoDBBackendWithURL(endpoint string) *dynamodbBackend {
	return &dynamodbBackend{baseURL: endpoint}
}

// Init starts or reuses a DynamoDB Local container and waits for it to be ready.
func (d *dynamodbBackend) Init(ctx context.Context, cfg config.AWSAuthConfig, deps service.ServiceDeps) error {
	hostPort, err := deps.PortAllocator.Allocate(8000, "dynamodb")
	if err != nil {
		return fmt.Errorf("dynamodb: allocate port: %w", err)
	}
	d.hostPort = hostPort
	d.baseURL = fmt.Sprintf("http://localhost:%d", hostPort)
	d.runner = deps.DockerClient
	d.portAlloc = deps.PortAllocator

	containerID, err := d.findOrCreateContainer(ctx, cfg, deps, hostPort)
	if err != nil {
		deps.PortAllocator.Release(hostPort)
		return err
	}

	d.containerID = containerID

	if err := d.waitHealthy(ctx); err != nil {
		return fmt.Errorf("dynamodb: health check failed: %w", err)
	}

	return nil
}

// findOrCreateContainer reuses an existing DynamoDB Local container or creates a new one.
func (d *dynamodbBackend) findOrCreateContainer(
	ctx context.Context,
	_ config.AWSAuthConfig,
	_ service.ServiceDeps,
	hostPort int,
) (string, error) {
	return d.runner.RunContainer(ctx, docker.ContainerConfig{
		Image: dynamodbImage,
		Name:  dynamodbContainerName,
		Labels: map[string]string{
			docker.LabelService: dynamodbServiceLabel,
		},
		Env: map[string]string{},
		Ports: map[string]string{
			dynamodbContainerPort: strconv.Itoa(hostPort),
		},
		Cmd: []string{"-jar", "DynamoDBLocal.jar", "-sharedDb", "-inMemory"},
	})
}

// Shutdown stops and removes the DynamoDB Local container, then releases the allocated port.
func (d *dynamodbBackend) Shutdown(ctx context.Context) error {
	if d.containerID == "" {
		return nil
	}

	if err := d.runner.StopContainer(ctx, d.containerID, nil); err != nil {
		return fmt.Errorf("dynamodb: stop container: %w", err)
	}

	if err := d.runner.RemoveContainer(ctx, d.containerID); err != nil {
		return fmt.Errorf("dynamodb: remove container: %w", err)
	}

	d.portAlloc.Release(d.hostPort)
	d.containerID = ""

	return nil
}

// Health checks whether DynamoDB Local is ready by calling its health endpoint.
func (d *dynamodbBackend) Health(ctx context.Context) service.HealthStatus {
	if err := d.callHealthEndpoint(ctx); err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// waitHealthy polls the DynamoDB Local health endpoint until it responds or the attempt limit is reached.
func (d *dynamodbBackend) waitHealthy(ctx context.Context) error {
	for i := 0; i < healthCheckMaxTries; i++ {
		if err := d.callHealthEndpoint(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("dynamodb local did not become ready after %d attempts", healthCheckMaxTries)
}

// callHealthEndpoint performs a single HTTP POST to the DynamoDB Local ListTables endpoint.
func (d *dynamodbBackend) callHealthEndpoint(ctx context.Context) error {
	body := bytes.NewBufferString("{}")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.ListTables")

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
