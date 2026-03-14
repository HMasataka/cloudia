package docker

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

// CreateVolume creates a volume with managed labels.
func (c *Client) CreateVolume(ctx context.Context, name string) error {
	_, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: ManagedLabels(name, "docker"),
	})
	return err
}

// RemoveVolume removes the named volume.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	return c.cli.VolumeRemove(ctx, name, true)
}

// ListManagedVolumes returns all volumes labelled with cloudia.managed=true.
func (c *Client) ListManagedVolumes(ctx context.Context) ([]*volume.Volume, error) {
	f := filters.NewArgs(filters.Arg("label", LabelManaged+"=true"))
	resp, err := c.cli.VolumeList(ctx, volume.ListOptions{Filters: f})
	if err != nil {
		return nil, err
	}
	return resp.Volumes, nil
}
