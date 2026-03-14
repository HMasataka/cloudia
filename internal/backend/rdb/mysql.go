package rdb

import (
	"fmt"
	"net"
	"time"
)

const (
	mysqlImage = "mysql:8.0"
)

// MySQLEngine implements the Engine interface for MySQL 8.0.
type MySQLEngine struct{}

// Image returns the Docker image for MySQL.
func (e *MySQLEngine) Image() string {
	return mysqlImage
}

// DefaultPort returns the default MySQL port.
func (e *MySQLEngine) DefaultPort() string {
	return "3306"
}

// ContainerName returns the Docker container name for MySQL.
func (e *MySQLEngine) ContainerName() string {
	return "cloudia-mysql"
}

// Env returns the environment variables required to initialise MySQL.
func (e *MySQLEngine) Env(rootPassword string) []string {
	return []string{
		"MYSQL_ROOT_PASSWORD=" + rootPassword,
	}
}

// HealthCheck performs a raw TCP dial to verify MySQL is accepting connections.
func (e *MySQLEngine) HealthCheck(host, port string) error {
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return fmt.Errorf("rdb: mysql health check: %w", err)
	}
	conn.Close()
	return nil
}
