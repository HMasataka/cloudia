package docker

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

// CreateNetwork creates a bridge network with managed labels and returns its ID.
func (c *Client) CreateNetwork(ctx context.Context, name string) (string, error) {
	resp, err := c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: ManagedLabels(name, "docker"),
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// RemoveNetwork removes the network with the given ID.
func (c *Client) RemoveNetwork(ctx context.Context, networkID string) error {
	return c.cli.NetworkRemove(ctx, networkID)
}

// ListManagedNetworks returns all networks labelled with cloudia.managed=true.
func (c *Client) ListManagedNetworks(ctx context.Context) ([]network.Summary, error) {
	f := filters.NewArgs(filters.Arg("label", LabelManaged+"=true"))
	return c.cli.NetworkList(ctx, network.ListOptions{Filters: f})
}
