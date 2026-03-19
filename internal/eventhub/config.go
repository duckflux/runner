package eventhub

// Config holds the event hub backend selection and per-backend options.
type Config struct {
	// Backend selects the pub/sub implementation: "memory" (default), "nats", or "redis".
	Backend string

	NATS  NATSConfig
	Redis RedisConfig
}

// NATSConfig holds connection parameters for the NATS JetStream backend.
type NATSConfig struct {
	// URL is the NATS server URL (e.g. "nats://localhost:4222").
	URL string
	// StreamName is the JetStream stream name (default: "duckflux-events").
	StreamName string
}

// RedisConfig holds connection parameters for the Redis Streams backend.
type RedisConfig struct {
	// Addr is the Redis server address (default: "localhost:6379").
	Addr string
	// DB is the Redis database number (default: 0).
	DB int
	// ConsumerGroup is the Redis consumer group name.
	// Defaults to "duckflux" when empty.
	ConsumerGroup string
}
