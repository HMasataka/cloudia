package docker

import (
	"context"

	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// Client wraps the Docker SDK client with a logger.
type Client struct {
	cli    *client.Client
	logger *zap.Logger
}

// NewClient creates a new Docker Client using environment configuration.
func NewClient(logger *zap.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli, logger: logger}, nil
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
func (c *Client) CleanupOrphans(ctx context.Context) error {
	containers, err := c.ListManagedContainers(ctx)
	if err != nil {
		return err
	}
	for _, ctr := range containers {
		if err := c.RemoveContainer(ctx, ctr.ID); err != nil {
			c.logger.Warn("failed to remove container", zap.String("id", ctr.ID), zap.Error(err))
		}
	}

	networks, err := c.ListManagedNetworks(ctx)
	if err != nil {
		return err
	}
	for _, net := range networks {
		if err := c.RemoveNetwork(ctx, net.ID); err != nil {
			c.logger.Warn("failed to remove network", zap.String("id", net.ID), zap.Error(err))
		}
	}

	volumes, err := c.ListManagedVolumes(ctx)
	if err != nil {
		return err
	}
	for _, vol := range volumes {
		if err := c.RemoveVolume(ctx, vol.Name); err != nil {
			c.logger.Warn("failed to remove volume", zap.String("name", vol.Name), zap.Error(err))
		}
	}

	return nil
}
