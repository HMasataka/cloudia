package rdb

import (
	"fmt"
	"net"
	"time"
)

const (
	postgresImage = "postgres:16"
)

// PostgreSQLEngine implements the Engine interface for PostgreSQL 16.
type PostgreSQLEngine struct{}

// Image returns the Docker image for PostgreSQL.
func (e *PostgreSQLEngine) Image() string {
	return postgresImage
}

// DefaultPort returns the default PostgreSQL port.
func (e *PostgreSQLEngine) DefaultPort() string {
	return "5432"
}

// ContainerName returns the Docker container name for PostgreSQL.
func (e *PostgreSQLEngine) ContainerName() string {
	return "cloudia-postgres"
}

// Env returns the environment variables required to initialise PostgreSQL.
func (e *PostgreSQLEngine) Env(rootPassword string) []string {
	return []string{
		"POSTGRES_USER=postgres",
		"POSTGRES_PASSWORD=" + rootPassword,
	}
}

// HealthCheck performs a raw TCP dial to verify PostgreSQL is accepting connections.
func (e *PostgreSQLEngine) HealthCheck(host, port string) error {
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return fmt.Errorf("rdb: postgres health check: %w", err)
	}
	conn.Close()
	return nil
}
