package logging_test

import (
	"testing"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/logging"
)

func TestNewLogger_JSONFormat(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
	}

	logger, err := logging.NewLogger(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}
	_ = logger.Sync()
}

func TestNewLogger_ConsoleFormat(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "console",
	}

	logger, err := logging.NewLogger(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}
	_ = logger.Sync()
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "invalid-level",
		Format: "json",
	}

	logger, err := logging.NewLogger(cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if logger != nil {
		t.Fatal("expected logger to be nil on error")
	}
}
