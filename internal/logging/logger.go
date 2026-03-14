package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/HMasataka/cloudia/internal/config"
)

// NewLogger は設定に基づいた zap.Logger を生成して返します。
// zap.ReplaceGlobals() は使用しません（明示的な DI を優先）。
func NewLogger(cfg config.LoggingConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("logging: invalid log level %q: %w", cfg.Level, err)
	}

	var zapCfg zap.Config
	switch cfg.Format {
	case "json":
		zapCfg = zap.NewProductionConfig()
	case "console":
		zapCfg = zap.NewDevelopmentConfig()
	default:
		zapCfg = zap.NewProductionConfig()
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.OutputPaths = []string{"stderr"}
	zapCfg.ErrorOutputPaths = []string{"stderr"}

	logger, err := zapCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("logging: failed to build logger: %w", err)
	}

	return logger, nil
}
