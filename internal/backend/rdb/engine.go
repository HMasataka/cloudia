package rdb

// Engine defines the Strategy interface for different relational database engines.
// Adding a new engine (e.g. PostgreSQL) only requires implementing this interface;
// no changes to backend.go are needed.
type Engine interface {
	// Image returns the Docker image to use for this engine.
	Image() string

	// DefaultPort returns the default port the engine listens on inside the container.
	DefaultPort() string

	// Env returns the environment variables to set on the container.
	Env(rootPassword string) []string

	// HealthCheck verifies the engine is accepting connections on the given host and port.
	HealthCheck(host, port string) error
}
