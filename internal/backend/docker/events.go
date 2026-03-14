package docker

import (
	"context"

	"github.com/docker/docker/api/types/events"
	"go.uber.org/zap"
)

// EventCallback is a function that handles a Docker event message.
type EventCallback func(events.Message)

// WatchEvents starts watching the Docker event stream in a goroutine and calls
// callback for each received event. It returns an error if the initial stream
// cannot be established. The goroutine exits when ctx is cancelled or the
// Docker daemon closes the stream.
func (c *Client) WatchEvents(ctx context.Context, callback EventCallback) error {
	msgCh, errCh := c.cli.Events(ctx, events.ListOptions{})

	go func() {
		for {
			select {
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				callback(msg)
			case err, ok := <-errCh:
				if !ok {
					return
				}
				if err != nil {
					c.logger.Error("docker event stream error", zap.Error(err))
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
