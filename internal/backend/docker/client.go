package docker

import (
	"context"

	"github.com/HMasataka/cloudia/internal/config"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// Client wraps the Docker SDK client with a logger.
type Client struct {
	cli         *client.Client
	logger      *zap.Logger
	networkName string
	labelPrefix string
}

// NewClient creates a new Docker Client using the provided DockerConfig.
func NewClient(cfg config.DockerConfig, logger *zap.Logger) (*Client, error) {
	opts := []client.Opt{client.FromEnv}
	if cfg.APIVersion != "" {
		opts = append(opts, client.WithVersion(cfg.APIVersion))
	} else {
		opts = append(opts, client.WithAPIVersionNegotiation())
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		cli:         cli,
		logger:      logger,
		networkName: cfg.NetworkName,
		labelPrefix: cfg.LabelPrefix,
	}, nil
}

// Ping verifies that the Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Close closes the underlying Docker client connection.
func (c *Client) Close() error {
	return c.cli.Close()
}

// CleanupOrphans removes all cloudia-managed containers, networks, and volumes.
// It returns the total number of resources removed.
func (c *Client) CleanupOrphans(ctx context.Context) (int, error) {
	removed := 0

	containers, err := c.ListManagedContainers(ctx)
	if err != nil {
		return removed, err
	}
	for _, ctr := range containers {
		if err := c.RemoveContainer(ctx, ctr.ID); err != nil {
			c.logger.Warn("failed to remove container", zap.String("id", ctr.ID), zap.Error(err))
		} else {
			removed++
		}
	}

	networks, err := c.ListManagedNetworks(ctx)
	if err != nil {
		return removed, err
	}
	for _, net := range networks {
		if err := c.RemoveNetwork(ctx, net.ID); err != nil {
			c.logger.Warn("failed to remove network", zap.String("id", net.ID), zap.Error(err))
		} else {
			removed++
		}
	}

	volumes, err := c.ListManagedVolumes(ctx)
	if err != nil {
		return removed, err
	}
	for _, vol := range volumes {
		if err := c.RemoveVolume(ctx, vol.Name); err != nil {
			c.logger.Warn("failed to remove volume", zap.String("name", vol.Name), zap.Error(err))
		} else {
			removed++
		}
	}

	return removed, nil
}
