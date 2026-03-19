// Package eventhub provides the event hub abstraction used by the duckflux
// runner to implement emit and wait.event across all supported backends.
//
// The Hub wraps a Watermill Publisher/Subscriber pair. Three backends are
// supported: "memory" (GoChannel, default), "nats" (NATS JetStream), and
// "redis" (Redis Streams). Backend is selected via Config.Backend.
package eventhub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// Hub is the central event hub. It owns the Watermill lifecycle and exposes
// typed Publish/Subscribe methods used by the engine and participants.
//
// Hub is safe for concurrent use.
type Hub struct {
	publisher  message.Publisher
	subscriber message.Subscriber
	config     Config
	// closers are called in order by Close. Using a slice avoids calling
	// Close twice on the GoChannel backend where pub and sub are the same object.
	closers []func() error
}

// New creates a Hub for the backend specified in cfg.Backend.
// An empty Backend defaults to "memory" (GoChannel).
func New(cfg Config) (*Hub, error) {
	switch cfg.Backend {
	case "", "memory":
		return newMemoryHub(cfg)
	case "nats":
		return newNATSHub(cfg)
	case "redis":
		return newRedisHub(cfg)
	default:
		return nil, fmt.Errorf("eventhub: unknown backend %q (want memory, nats, or redis)", cfg.Backend)
	}
}

// Publish marshals payload into an EventEnvelope and publishes it to the
// topic equal to event. It is safe to call concurrently.
func (h *Hub) Publish(_ context.Context, event string, payload any) error {
	data, err := json.Marshal(EventEnvelope{Name: event, Payload: payload})
	if err != nil {
		return fmt.Errorf("eventhub: marshaling event %q: %w", event, err)
	}
	msg := message.NewMessage(watermill.NewUUID(), data)
	if err := h.publisher.Publish(event, msg); err != nil {
		return fmt.Errorf("eventhub: publishing event %q: %w", event, err)
	}
	return nil
}

// PublishAndWaitAck publishes an event and waits at most timeout for the
// publish to be confirmed. For the GoChannel backend this is inherent
// (synchronous channel delivery). For NATS JetStream and Redis Streams a
// successful publish call means the broker persisted the message.
//
// If timeout elapses before the publish completes, a non-nil error wrapping
// context.DeadlineExceeded is returned.
func (h *Hub) PublishAndWaitAck(ctx context.Context, event string, payload any, timeout time.Duration) error {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- h.Publish(tCtx, event, payload)
	}()

	select {
	case err := <-done:
		return err
	case <-tCtx.Done():
		return fmt.Errorf("eventhub: ack timeout for event %q: %w", event, tCtx.Err())
	}
}

// Subscribe subscribes to the given event topic and returns a channel that
// delivers decoded EventEnvelopes. The returned cancel function must be called
// when the subscription is no longer needed to release resources.
//
// The channel is closed when cancel is called or ctx is cancelled.
func (h *Hub) Subscribe(ctx context.Context, event string) (<-chan EventEnvelope, func(), error) {
	subCtx, cancel := context.WithCancel(ctx)

	msgs, err := h.subscriber.Subscribe(subCtx, event)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("eventhub: subscribing to %q: %w", event, err)
	}

	ch := make(chan EventEnvelope, 16)
	go func() {
		defer close(ch)
		for msg := range msgs {
			var env EventEnvelope
			if err := json.Unmarshal(msg.Payload, &env); err != nil {
				msg.Nack()
				continue
			}
			msg.Ack()
			select {
			case ch <- env:
			case <-subCtx.Done():
				return
			}
		}
	}()

	return ch, cancel, nil
}

// Close shuts down all backend connections. It is safe to call once.
func (h *Hub) Close() error {
	var firstErr error
	for _, fn := range h.closers {
		if err := fn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
