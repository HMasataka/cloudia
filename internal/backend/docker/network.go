package docker

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

// CreateNetwork creates a bridge network with managed labels and returns its ID.
// If cidr is non-empty, IPAM is configured with the given subnet.
func (c *Client) CreateNetwork(ctx context.Context, name, cidr string) (string, error) {
	opts := network.CreateOptions{
		Driver: "bridge",
		Labels: ManagedLabels(name, "docker", "", ""),
	}
	if cidr != "" {
		opts.IPAM = &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{
				{Subnet: cidr},
			},
		}
	}
	resp, err := c.cli.NetworkCreate(ctx, name, opts)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// EnsureNetwork creates a bridge network with the given name if it does not already exist.
// It is idempotent: if a network with the same name already exists, it returns its ID without error.
func (c *Client) EnsureNetwork(ctx context.Context, name string) (string, error) {
	f := filters.NewArgs(filters.Arg("name", name))
	existing, err := c.cli.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return "", err
	}
	for _, n := range existing {
		if n.Name == name {
			return n.ID, nil
		}
	}
	return c.CreateNetwork(ctx, name, "")
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
