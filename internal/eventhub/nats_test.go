package eventhub_test

import (
	"context"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsJS "github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/duckflux/runner/internal/eventhub"
)

// startNATSContainer starts a NATS container (JetStream is enabled by default
// by the nats module). Skips the test if Docker is not available.
func startNATSContainer(t *testing.T) (natsURL string, terminate func()) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	ctr, err := tcnats.Run(ctx, "nats:2.10")
	if err != nil {
		t.Fatalf("starting NATS container: %v", err)
	}

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		t.Fatalf("getting NATS connection string: %v", err)
	}

	return url, func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			t.Logf("terminating NATS container: %v", err)
		}
	}
}

// ensureStream pre-creates a JetStream stream so that EphemeralConsumer (which
// calls js.Stream — a get, not create) can find it when Subscribe is called.
//
// JetStream stream names cannot contain dots, so the topic is sanitized
// (dots replaced with underscores) for the stream NAME while the original
// topic string is kept as the subject. NATS routes by subject, so the Hub
// still publishes/subscribes using the original dot-notation event name.
func ensureStream(t *testing.T, natsURL, topic string) {
	t.Helper()
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		t.Fatalf("connecting to NATS for stream setup: %v", err)
	}
	defer nc.Close()

	js, err := natsJS.New(nc)
	if err != nil {
		t.Fatalf("creating JetStream context: %v", err)
	}

	streamName := strings.ReplaceAll(topic, ".", "_")
	_, err = js.CreateOrUpdateStream(context.Background(), natsJS.StreamConfig{
		Name:     streamName,
		Subjects: []string{topic},
	})
	if err != nil {
		t.Fatalf("creating JetStream stream %q (subject %q): %v", streamName, topic, err)
	}
}

func TestNATSPublishSubscribeRoundTrip(t *testing.T) {
	t.Parallel()
	natsURL, terminate := startNATSContainer(t)
	defer terminate()

	const topic = "nats.roundtrip"
	ensureStream(t, natsURL, topic)

	hub, err := eventhub.New(eventhub.Config{
		Backend: "nats",
		NATS:    eventhub.NATSConfig{URL: natsURL},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()
	ch, cancel, err := hub.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if err := hub.Publish(ctx, topic, map[string]any{"hello": "nats"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case env := <-ch:
		if env.Name != topic {
			t.Errorf("expected Name %q, got %q", topic, env.Name)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestNATSPersistentReplay is skipped: NATS ephemeral consumers start from the
// latest offset and do not replay messages published before the consumer was created.
func TestNATSPersistentReplay(t *testing.T) {
	t.Skip("NATS JetStream ephemeral consumers do not replay pre-subscribe messages")
}

func TestNATSPublishAndWaitAck(t *testing.T) {
	t.Parallel()
	natsURL, terminate := startNATSContainer(t)
	defer terminate()

	const topic = "nats.ack"
	ensureStream(t, natsURL, topic)

	hub, err := eventhub.New(eventhub.Config{
		Backend: "nats",
		NATS:    eventhub.NATSConfig{URL: natsURL},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()
	ch, cancel, err := hub.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	go func() {
		for range ch {
		}
	}()

	if err := hub.PublishAndWaitAck(ctx, topic, "payload", 10*time.Second); err != nil {
		t.Fatalf("PublishAndWaitAck: %v", err)
	}
}

func TestNATSMultipleSubscribers(t *testing.T) {
	t.Parallel()
	natsURL, terminate := startNATSContainer(t)
	defer terminate()

	const topic = "nats.multi"
	ensureStream(t, natsURL, topic)

	// Two separate Hubs = two independent ephemeral JetStream consumers.
	// Each receives its own copy of every published message (fan-out).
	makeHub := func() *eventhub.Hub {
		h, err := eventhub.New(eventhub.Config{
			Backend: "nats",
			NATS:    eventhub.NATSConfig{URL: natsURL},
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return h
	}

	hub1 := makeHub()
	defer hub1.Close()
	hub2 := makeHub()
	defer hub2.Close()

	ctx := context.Background()
	ch1, cancel1, err := hub1.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe hub1: %v", err)
	}
	defer cancel1()

	ch2, cancel2, err := hub2.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe hub2: %v", err)
	}
	defer cancel2()

	if err := hub1.Publish(ctx, topic, 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	timeout := time.After(10 * time.Second)
	for i, ch := range []<-chan eventhub.EventEnvelope{ch1, ch2} {
		select {
		case env := <-ch:
			if env.Name != topic {
				t.Errorf("subscriber %d: unexpected event name: %q", i+1, env.Name)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for event on subscriber %d", i+1)
		}
	}
}
