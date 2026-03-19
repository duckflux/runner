package eventhub

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	wmnats "github.com/ThreeDotsLabs/watermill-nats/v2/pkg/jetstream"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// newNATSHub creates a Hub backed by NATS JetStream.
//
// The subscriber uses an ephemeral consumer backed by dotSafeEphemeralConsumer,
// which resolves the stream by subject (via StreamNameBySubject) rather than
// using the topic string directly as the stream name. This allows event names
// with dots (e.g. "payment.processed") — which are valid NATS subjects but
// invalid JetStream stream names — to work transparently.
//
// cfg.NATS.URL must be non-empty. Streams must be pre-created (e.g. via
// eventhub.EnsureNATSStream) before publishing or subscribing.
func newNATSHub(cfg Config) (*Hub, error) {
	if cfg.NATS.URL == "" {
		return nil, fmt.Errorf("eventhub nats: NATS.URL must be set")
	}

	logger := watermill.NopLogger{}

	pub, err := wmnats.NewPublisher(wmnats.PublisherConfig{
		URL:    cfg.NATS.URL,
		Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("eventhub nats: creating publisher: %w", err)
	}

	sub, err := wmnats.NewSubscriber(wmnats.SubscriberConfig{
		URL:                 cfg.NATS.URL,
		Logger:              logger,
		ResourceInitializer: dotSafeEphemeralConsumer(),
	})
	if err != nil {
		_ = pub.Close()
		return nil, fmt.Errorf("eventhub nats: creating subscriber: %w", err)
	}

	return &Hub{
		publisher:  pub,
		subscriber: sub,
		config:     cfg,
		closers:    []func() error{pub.Close, sub.Close},
	}, nil
}

// dotSafeEphemeralConsumer returns a ResourceInitializer that creates an
// ephemeral JetStream consumer by looking up the stream via
// StreamNameBySubject. This allows topics with dots (valid NATS subjects)
// to be used even though JetStream stream names cannot contain dots.
func dotSafeEphemeralConsumer() wmnats.ResourceInitializer {
	return func(ctx context.Context, js natsjs.JetStream, topic string) (
		natsjs.Consumer,
		func(context.Context, watermill.LoggerAdapter),
		error,
	) {
		streamName, err := js.StreamNameBySubject(ctx, topic)
		if err != nil {
			return nil, nil, fmt.Errorf("eventhub nats: no stream found for subject %q: %w", topic, err)
		}

		stream, err := js.Stream(ctx, streamName)
		if err != nil {
			return nil, nil, fmt.Errorf("eventhub nats: getting stream %q: %w", streamName, err)
		}

		consumerName := "watermill__" + watermill.NewShortUUID()
		cfg := natsjs.ConsumerConfig{
			Name:      consumerName,
			AckPolicy: natsjs.AckExplicitPolicy,
		}

		consumer, err := stream.CreateOrUpdateConsumer(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("eventhub nats: creating consumer for %q: %w", topic, err)
		}

		cleanup := func(cctx context.Context, logger watermill.LoggerAdapter) {
			if deleteErr := stream.DeleteConsumer(cctx, consumerName); deleteErr != nil {
				logger.Error("failed to delete ephemeral consumer", deleteErr, watermill.LogFields{})
			}
		}

		return consumer, cleanup, nil
	}
}
