package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// ContainerInfo holds runtime state information for a container.
type ContainerInfo struct {
	State     string
	IPAddress string
	OOMKilled bool
}

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
	Binds       []string
	Privileged  bool
}

// RunContainer pulls an image, creates a container, and starts it.
// It returns the container ID on success.
// Port conflict errors are returned immediately so callers can reallocate ports and retry.
func (c *Client) RunContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	if err := c.PullImageWithRetry(ctx, cfg.Image); err != nil {
		return "", err
	}
	return c.createAndStartContainer(ctx, cfg)
}

// createAndStartContainer creates and starts a container from the given config.
func (c *Client) createAndStartContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
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
		Binds:        cfg.Binds,
		PortBindings: portBindings,
		Privileged:   cfg.Privileged,
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
		portBindings[p] = []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: hostPort}}
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

// StartContainer starts a stopped container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// PauseContainer pauses a running container.
func (c *Client) PauseContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerPause(ctx, containerID)
}

// UnpauseContainer unpauses a paused container.
func (c *Client) UnpauseContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerUnpause(ctx, containerID)
}

// InspectContainer returns state information for a container.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (ContainerInfo, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return ContainerInfo{}, err
	}
	var ipAddress string
	if info.NetworkSettings != nil {
		ipAddress = info.NetworkSettings.IPAddress
		if ipAddress == "" {
			for _, n := range info.NetworkSettings.Networks {
				if n != nil && n.IPAddress != "" {
					ipAddress = n.IPAddress
					break
				}
			}
		}
	}
	state := ""
	oomKilled := false
	if info.State != nil {
		state = info.State.Status
		oomKilled = info.State.OOMKilled
	}
	return ContainerInfo{
		State:     state,
		IPAddress: ipAddress,
		OOMKilled: oomKilled,
	}, nil
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

// ContainerLogs returns the last N lines of logs from the specified container.
// If lines is 0 or negative, it defaults to 100. If lines exceeds 10000, it is capped at 10000.
// Non-UTF-8 bytes in the output are replaced with the Unicode replacement character.
func (c *Client) ContainerLogs(ctx context.Context, containerID string, lines int) (string, error) {
	const defaultLines = 100
	const maxLines = 10000

	if lines <= 0 {
		lines = defaultLines
	}
	if lines > maxLines {
		lines = maxLines
	}

	tail := fmt.Sprintf("%d", lines)
	rc, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return "", fmt.Errorf("container logs: %w", err)
	}
	defer rc.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil {
		// Fall back to raw read if stdcopy fails (e.g. non-multiplexed stream).
		rc2, err2 := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       tail,
		})
		if err2 != nil {
			return "", fmt.Errorf("container logs: %w", err2)
		}
		defer rc2.Close()
		raw, err2 := io.ReadAll(rc2)
		if err2 != nil {
			return "", fmt.Errorf("container logs read: %w", err2)
		}
		return strings.ToValidUTF8(string(raw), "\uFFFD"), nil
	}

	combined := stdout.String() + stderr.String()
	return strings.ToValidUTF8(combined, "\uFFFD"), nil
}

// ExecInContainer runs cmd inside the specified container and returns the combined stdout output.
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) ([]byte, error) {
	execResp, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := c.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader); err != nil {
		return nil, fmt.Errorf("exec read output: %w", err)
	}

	return stdout.Bytes(), nil
}
