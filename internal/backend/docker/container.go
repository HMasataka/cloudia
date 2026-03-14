package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
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

	exposedPorts, portBindings, err := buildPortMappings(cfg.Ports)
	if err != nil {
		return "", err
	}

	containerCfg := &container.Config{
		Image:        cfg.Image,
		Labels:       labels,
		Env:          env,
		Cmd:          cfg.Cmd,
		ExposedPorts: exposedPorts,
	}

	hostCfg := &container.HostConfig{
		PortBindings: portBindings,
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

// buildPortMappings converts a Ports map ("containerPort/proto" -> "hostPort") into
// Docker's ExposedPorts and PortBindings structures.
func buildPortMappings(ports map[string]string) (nat.PortSet, nat.PortMap, error) {
	exposedPorts := make(nat.PortSet, len(ports))
	portBindings := make(nat.PortMap, len(ports))

	for containerPort, hostPort := range ports {
		p, err := nat.NewPort("tcp", containerPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid container port %q: %w", containerPort, err)
		}
		exposedPorts[p] = struct{}{}
		portBindings[p] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	return exposedPorts, portBindings, nil
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

// FindContainerByServiceLabel returns the first running container matching the given service label value.
// Returns nil, nil when no matching container is found.
func (c *Client) FindContainerByServiceLabel(ctx context.Context, serviceValue string) (*container.Summary, error) {
	f := filters.NewArgs(
		filters.Arg("label", LabelManaged+"=true"),
		filters.Arg("label", LabelService+"="+serviceValue),
	)
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     false,
		Filters: f,
	})
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, nil
	}
	return &containers[0], nil
}
