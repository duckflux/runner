package eventhub

import (
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/redis/go-redis/v9"
)

const defaultRedisAddr          = "localhost:6379"
const defaultRedisConsumerGroup = "duckflux"

// newRedisHub creates a Hub backed by Redis Streams.
//
// Stream key mapping: event name → Redis stream key (Watermill uses the topic
// as the stream key directly). Consumer group defaults to "duckflux" when
// cfg.Redis.ConsumerGroup is empty.
//
// This function requires cfg.Redis.Addr to be non-empty (or uses the default
// "localhost:6379" if Addr is blank).
func newRedisHub(cfg Config) (*Hub, error) {
	addr := cfg.Redis.Addr
	if addr == "" {
		addr = defaultRedisAddr
	}
	consumerGroup := cfg.Redis.ConsumerGroup
	if consumerGroup == "" {
		consumerGroup = defaultRedisConsumerGroup
	}

	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   cfg.Redis.DB,
	})

	logger := watermill.NopLogger{}

	pub, err := redisstream.NewPublisher(
		redisstream.PublisherConfig{Client: client},
		logger,
	)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("eventhub redis: creating publisher: %w", err)
	}

	sub, err := redisstream.NewSubscriber(
		redisstream.SubscriberConfig{
			Client:        client,
			ConsumerGroup: consumerGroup,
		},
		logger,
	)
	if err != nil {
		_ = pub.Close()
		_ = client.Close()
		return nil, fmt.Errorf("eventhub redis: creating subscriber: %w", err)
	}

	return &Hub{
		publisher:  pub,
		subscriber: sub,
		config:     cfg,
		closers:    []func() error{pub.Close, sub.Close, client.Close},
	}, nil
}
