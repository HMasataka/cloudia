package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// ContainerConfig holds configuration for creating and running a container.
type ContainerConfig struct {
	Image       string
	Name        string
	Labels      map[string]string
	Env         map[string]string
	Ports       map[string]string
	Network     string
	CPULimit    int64
	MemoryLimit int64
	Cmd         []string
}

// RunContainer pulls an image, creates a container, and starts it.
// It returns the container ID on success.
func (c *Client) RunContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	reader, err := c.cli.ImagePull(ctx, cfg.Image, image.PullOptions{})
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return "", err
	}

	// Merge managed labels with caller-supplied labels.
	labels := make(map[string]string, len(cfg.Labels)+1)
	for k, v := range cfg.Labels {
		labels[k] = v
	}
	labels[LabelManaged] = "true"

	// Build env slice.
	env := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	containerCfg := &container.Config{
		Image:  cfg.Image,
		Labels: labels,
		Env:    env,
		Cmd:    cfg.Cmd,
	}

	hostCfg := &container.HostConfig{
		Resources: container.Resources{
			NanoCPUs: cfg.CPULimit,
			Memory:   cfg.MemoryLimit,
		},
	}
	if cfg.Network != "" {
		hostCfg.NetworkMode = container.NetworkMode(cfg.Network)
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", err
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}

	return resp.ID, nil
}

// StopContainer stops a running container with an optional timeout.
func (c *Client) StopContainer(ctx context.Context, containerID string, timeout *int) error {
	return c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: timeout})
}

// RemoveContainer force-removes a container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// ListManagedContainers returns all containers labelled with cloudia.managed=true.
func (c *Client) ListManagedContainers(ctx context.Context) ([]container.Summary, error) {
	f := filters.NewArgs(filters.Arg("label", LabelManaged+"=true"))
	return c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: f,
	})
}
