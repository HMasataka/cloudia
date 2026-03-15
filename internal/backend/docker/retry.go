package docker

import (
	"context"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
	"go.uber.org/zap"
)

// retryConfig holds parameters for exponential backoff retries.
type retryConfig struct {
	maxAttempts int
	initialWait time.Duration
	multiplier  float64
	maxWait     time.Duration
	jitter      float64 // fraction (e.g. 0.25 for +/-25%)
}

var defaultImagePullRetry = retryConfig{
	maxAttempts: 3,
	initialWait: 1 * time.Second,
	multiplier:  2,
	maxWait:     8 * time.Second,
	jitter:      0.25,
}

// calcBackoff calculates the sleep duration for the given attempt (0-indexed).
func calcBackoff(cfg retryConfig, attempt int) time.Duration {
	wait := float64(cfg.initialWait)
	for i := 0; i < attempt; i++ {
		wait *= cfg.multiplier
		if wait > float64(cfg.maxWait) {
			wait = float64(cfg.maxWait)
			break
		}
	}
	// Apply jitter: +/- jitter fraction
	jitterRange := wait * cfg.jitter
	// rand.Float64() returns [0,1), we map to [-1, 1)
	jitter := (rand.Float64()*2 - 1) * jitterRange //nolint:gosec
	result := time.Duration(wait + jitter)
	if result < 0 {
		result = 0
	}
	return result
}

// PullImageWithRetry pulls a Docker image with exponential backoff retry.
// It retries up to maxAttempts times on transient errors.
func (c *Client) PullImageWithRetry(ctx context.Context, ref string) error {
	cfg := defaultImagePullRetry
	var lastErr error
	for attempt := 0; attempt < cfg.maxAttempts; attempt++ {
		if attempt > 0 {
			sleep := calcBackoff(cfg, attempt-1)
			c.logger.Info("retrying image pull",
				zap.String("image", ref),
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", cfg.maxAttempts),
				zap.Duration("wait", sleep),
				zap.Error(lastErr),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}

		reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
		if err != nil {
			lastErr = err
			continue
		}
		_, copyErr := io.Copy(io.Discard, reader)
		reader.Close()
		if copyErr != nil {
			lastErr = copyErr
			continue
		}
		return nil
	}
	return lastErr
}

// isPortConflictError reports whether err indicates a port binding conflict.
func isPortConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "port is already allocated") ||
		strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "bind: address already in use") ||
		strings.Contains(msg, "Bind for") && strings.Contains(msg, "failed")
}

