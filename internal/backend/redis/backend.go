package redis

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/service"
	"go.uber.org/zap"
)

const (
	redisImage          = "redis:7.4"
	redisContainerName  = "cloudia-redis"
	redisContainerPort  = "6379"
	redisServiceLabel   = "elasticache"
	redisNetwork        = "cloudia"
	healthCheckInterval = time.Second
	healthCheckMaxTries = 30
)

// RedisBackend manages the lifecycle of a Redis container.
type RedisBackend struct {
	runner      service.ContainerRunner
	logger      *zap.Logger
	containerID string
	host        string
	port        string
}

// NewRedisBackend creates a new RedisBackend with the given logger.
func NewRedisBackend(logger *zap.Logger) *RedisBackend {
	return &RedisBackend{
		logger: logger,
		host:   "localhost",
	}
}

// Init starts a Redis container and waits for it to be ready via PING/PONG health check.
func (r *RedisBackend) Init(ctx context.Context, deps service.ServiceDeps) error {
	hostPort, err := deps.PortAllocator.Allocate(6379, "redis")
	if err != nil {
		return fmt.Errorf("redis: allocate port: %w", err)
	}
	r.port = strconv.Itoa(hostPort)
	r.runner = deps.DockerClient

	containerID, err := r.runner.RunContainer(ctx, docker.ContainerConfig{
		Image: redisImage,
		Name:  redisContainerName,
		Labels: map[string]string{
			docker.LabelService: redisServiceLabel,
		},
		Ports: map[string]string{
			redisContainerPort: r.port,
		},
		Network: redisNetwork,
	})
	if err != nil {
		deps.PortAllocator.Release(hostPort)
		return fmt.Errorf("redis: run container: %w", err)
	}

	r.containerID = containerID

	if err := r.waitHealthy(ctx); err != nil {
		return fmt.Errorf("redis: health check failed: %w", err)
	}

	return nil
}

// Shutdown stops and removes the Redis container, then releases the allocated port.
func (r *RedisBackend) Shutdown(ctx context.Context, deps service.ServiceDeps) error {
	if r.containerID == "" {
		return nil
	}

	if err := r.runner.StopContainer(ctx, r.containerID, nil); err != nil {
		return fmt.Errorf("redis: stop container: %w", err)
	}

	if err := r.runner.RemoveContainer(ctx, r.containerID); err != nil {
		return fmt.Errorf("redis: remove container: %w", err)
	}

	port, err := strconv.Atoi(r.port)
	if err == nil {
		deps.PortAllocator.Release(port)
	}

	r.containerID = ""

	return nil
}

// Host returns the host address of the Redis container.
func (r *RedisBackend) Host() string {
	return r.host
}

// Port returns the host port of the Redis container.
func (r *RedisBackend) Port() string {
	return r.port
}

// waitHealthy polls the Redis PING endpoint until it responds or the attempt limit is reached.
func (r *RedisBackend) waitHealthy(ctx context.Context) error {
	for i := 0; i < healthCheckMaxTries; i++ {
		if err := r.ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("redis did not become ready after %d attempts", healthCheckMaxTries)
}

// ping sends a PING command over raw TCP and checks for +PONG response.
func (r *RedisBackend) ping(ctx context.Context) error {
	addr := net.JoinHostPort(r.host, r.port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := fmt.Fprint(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	line = strings.TrimSpace(line)
	if line != "+PONG" {
		return fmt.Errorf("unexpected redis response: %q", line)
	}

	return nil
}

// Health checks whether Redis is reachable via PING.
func (r *RedisBackend) Health(ctx context.Context) service.HealthStatus {
	if err := r.ping(ctx); err != nil {
		return service.HealthStatus{Healthy: false, Message: err.Error()}
	}
	return service.HealthStatus{Healthy: true, Message: "ok"}
}
