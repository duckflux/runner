package eventhub_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/duckflux/runner/internal/eventhub"
)

// startRedisContainer starts a Redis container and returns addr ("host:port").
// Skips the test if Docker is not available.
func startRedisContainer(t *testing.T) (addr string, terminate func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("starting Redis container: %v", err)
	}

	connStr, err := ctr.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		t.Fatalf("getting Redis connection string: %v", err)
	}

	// ConnectionString returns "redis://host:port"; strip the scheme.
	addr = strings.TrimPrefix(connStr, "redis://")

	return addr, func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Logf("terminating Redis container: %v", err)
		}
	}
}

func TestRedisPublishSubscribeRoundTrip(t *testing.T) {
	t.Parallel()
	addr, terminate := startRedisContainer(t)
	defer terminate()

	hub, err := eventhub.New(eventhub.Config{
		Backend: "redis",
		Redis:   eventhub.RedisConfig{Addr: addr, ConsumerGroup: "test-roundtrip"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()
	ch, cancel, err := hub.Subscribe(ctx, "redis.roundtrip")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if err := hub.Publish(ctx, "redis.roundtrip", map[string]any{"hello": "redis"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case env := <-ch:
		if env.Name != "redis.roundtrip" {
			t.Errorf("expected Name %q, got %q", "redis.roundtrip", env.Name)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestRedisPersistentReplay verifies that a subscriber created after a publish
// still receives the message. Redis Streams defaults OldestId="0" so consumer
// groups read from the beginning of the stream.
func TestRedisPersistentReplay(t *testing.T) {
	t.Parallel()
	addr, terminate := startRedisContainer(t)
	defer terminate()

	hub, err := eventhub.New(eventhub.Config{
		Backend: "redis",
		Redis:   eventhub.RedisConfig{Addr: addr, ConsumerGroup: "test-replay"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	// Publish BEFORE subscribing.
	if err := hub.Publish(ctx, "redis.pre", "before-sub"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ch, cancel, err := hub.Subscribe(ctx, "redis.pre")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	select {
	case env := <-ch:
		if env.Name != "redis.pre" {
			t.Errorf("expected Name %q, got %q", "redis.pre", env.Name)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout: Redis event not replayed after subscribe")
	}
}

func TestRedisPublishAndWaitAck(t *testing.T) {
	t.Parallel()
	addr, terminate := startRedisContainer(t)
	defer terminate()

	hub, err := eventhub.New(eventhub.Config{
		Backend: "redis",
		Redis:   eventhub.RedisConfig{Addr: addr, ConsumerGroup: "test-ack"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()
	ch, cancel, err := hub.Subscribe(ctx, "redis.ack")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	go func() {
		for range ch {
		}
	}()

	if err := hub.PublishAndWaitAck(ctx, "redis.ack", "payload", 10*time.Second); err != nil {
		t.Fatalf("PublishAndWaitAck: %v", err)
	}
}

func TestRedisMultipleSubscribers(t *testing.T) {
	t.Parallel()
	addr, terminate := startRedisContainer(t)
	defer terminate()

	// Hub.subscriber is a single redisstream.Subscriber (one ConsumerGroup).
	// Two Subscribe calls on the same Hub share the group → work-queue.
	// For fan-out: each subscriber needs its own Hub with a distinct ConsumerGroup.
	makeHub := func(group string) *eventhub.Hub {
		h, err := eventhub.New(eventhub.Config{
			Backend: "redis",
			Redis:   eventhub.RedisConfig{Addr: addr, ConsumerGroup: group},
		})
		if err != nil {
			t.Fatalf("New (group=%s): %v", group, err)
		}
		return h
	}

	hub1 := makeHub("test-multi-g1")
	defer hub1.Close()
	hub2 := makeHub("test-multi-g2")
	defer hub2.Close()

	ctx := context.Background()
	ch1, cancel1, err := hub1.Subscribe(ctx, "redis.multi")
	if err != nil {
		t.Fatalf("Subscribe hub1: %v", err)
	}
	defer cancel1()

	ch2, cancel2, err := hub2.Subscribe(ctx, "redis.multi")
	if err != nil {
		t.Fatalf("Subscribe hub2: %v", err)
	}
	defer cancel2()

	if err := hub1.Publish(ctx, "redis.multi", 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	timeout := time.After(10 * time.Second)
	for i, ch := range []<-chan eventhub.EventEnvelope{ch1, ch2} {
		select {
		case env := <-ch:
			if env.Name != "redis.multi" {
				t.Errorf("subscriber %d: unexpected event name: %q", i+1, env.Name)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for event on subscriber %d", i+1)
		}
	}
}
