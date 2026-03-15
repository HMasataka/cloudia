package rdb

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

const (
	rdbServiceLabel     = "rdb"
	rdbNetwork          = "cloudia"
	healthCheckMaxTries = 60
	healthCheckInterval = time.Second
	defaultRootPassword = "cloudia"
)

// RDBBackend manages the lifecycle of a relational database container.
// It delegates engine-specific details (image, port, env, healthcheck) to an Engine.
type RDBBackend struct {
	engine       Engine
	runner       service.ContainerRunner
	logger       *zap.Logger
	containerID  string
	host         string
	port         string
	rootPassword string
	portAlloc    service.PortAllocator
	hostPort     int
}

// NewRDBBackend creates a new RDBBackend with the given engine.
func NewRDBBackend(engine Engine, logger *zap.Logger) *RDBBackend {
	return &RDBBackend{
		engine: engine,
		logger: logger,
	}
}

// NewRDBBackendStub creates a pre-initialised RDBBackend with fixed host/port values.
// Use this in tests to avoid Docker dependencies.
func NewRDBBackendStub(engine Engine, host, port string, logger *zap.Logger) *RDBBackend {
	return &RDBBackend{
		engine: engine,
		logger: logger,
		host:   host,
		port:   port,
	}
}

// Init starts the database container and waits until it is healthy.
// The rootPassword used for the container is the hardcoded default "cloudia".
// Passwords supplied during CreateDBInstance are stored as metadata only.
func (b *RDBBackend) Init(ctx context.Context, deps service.ServiceDeps) error {
	b.rootPassword = defaultRootPassword
	b.runner = deps.DockerClient
	b.portAlloc = deps.PortAllocator
	b.host = "localhost"

	defaultPort, err := strconv.Atoi(b.engine.DefaultPort())
	if err != nil {
		defaultPort = 3306
	}

	hostPort, err := deps.PortAllocator.Allocate(defaultPort, b.engine.ContainerName())
	if err != nil {
		return fmt.Errorf("rdb: allocate port: %w", err)
	}
	b.hostPort = hostPort
	b.port = strconv.Itoa(hostPort)

	env := b.engine.Env(b.rootPassword)
	envMap := make(map[string]string, len(env))
	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				envMap[e[:i]] = e[i+1:]
				break
			}
		}
	}

	containerID, err := b.runner.RunContainer(ctx, docker.ContainerConfig{
		Image: b.engine.Image(),
		Name:  b.engine.ContainerName(),
		Labels: map[string]string{
			docker.LabelService: rdbServiceLabel,
		},
		Env: envMap,
		Ports: map[string]string{
			b.engine.DefaultPort(): b.port,
		},
		Network: rdbNetwork,
	})
	if err != nil {
		deps.PortAllocator.Release(hostPort)
		return fmt.Errorf("rdb: run container: %w", err)
	}
	b.containerID = containerID

	if err := b.waitHealthy(ctx); err != nil {
		return fmt.Errorf("rdb: health check failed: %w", err)
	}

	b.logger.Info("rdb backend ready", zap.String("host", b.host), zap.String("port", b.port))
	return nil
}

// waitHealthy polls the engine's HealthCheck until it succeeds or the attempt limit is reached.
func (b *RDBBackend) waitHealthy(ctx context.Context) error {
	for i := 0; i < healthCheckMaxTries; i++ {
		if err := b.engine.HealthCheck(b.host, b.port); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("rdb: container did not become ready after %d attempts", healthCheckMaxTries)
}

// Shutdown stops and removes the database container, then releases the allocated port.
func (b *RDBBackend) Shutdown(ctx context.Context) error {
	if b.containerID == "" {
		return nil
	}

	if err := b.runner.StopContainer(ctx, b.containerID, nil); err != nil {
		return fmt.Errorf("rdb: stop container: %w", err)
	}

	if err := b.runner.RemoveContainer(ctx, b.containerID); err != nil {
		return fmt.Errorf("rdb: remove container: %w", err)
	}

	b.portAlloc.Release(b.hostPort)
	b.containerID = ""

	return nil
}

// Health checks whether the database is accepting connections.
func (b *RDBBackend) Health(_ context.Context) service.HealthStatus {
	if err := b.engine.HealthCheck(b.host, b.port); err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	return service.HealthStatus{Healthy: true, Message: "ok"}
}

// Host returns the host address of the database container.
func (b *RDBBackend) Host() string {
	return b.host
}

// Port returns the host-mapped port of the database container.
func (b *RDBBackend) Port() string {
	return b.port
}

// RootPassword returns the root password used to initialise the database container.
func (b *RDBBackend) RootPassword() string {
	return b.rootPassword
}
