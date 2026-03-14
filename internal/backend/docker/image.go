package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/image"
)

// PullImage pulls a Docker image by reference, discarding the progress output.
func (c *Client) PullImage(ctx context.Context, ref string) error {
	reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}
