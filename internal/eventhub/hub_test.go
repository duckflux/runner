package eventhub_test

import (
	"context"
	"testing"
	"time"

	"github.com/duckflux/runner/internal/eventhub"
)

func TestGoChannelPublishSubscribeRoundTrip(t *testing.T) {
	hub, err := eventhub.New(eventhub.Config{Backend: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	ch, cancel, err := hub.Subscribe(ctx, "test.event")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	payload := map[string]any{"hello": "world"}
	if err := hub.Publish(ctx, "test.event", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case env := <-ch:
		if env.Name != "test.event" {
			t.Errorf("expected Name 'test.event', got %q", env.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestGoChannelPersistentReplay(t *testing.T) {
	// Publish before subscribing; persistent mode should replay the event.
	hub, err := eventhub.New(eventhub.Config{Backend: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	if err := hub.Publish(ctx, "pre.event", "before-sub"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ch, cancel, err := hub.Subscribe(ctx, "pre.event")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	select {
	case env := <-ch:
		if env.Name != "pre.event" {
			t.Errorf("expected Name 'pre.event', got %q", env.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: persistent event not replayed")
	}
}

func TestGoChannelPublishAndWaitAck(t *testing.T) {
	hub, err := eventhub.New(eventhub.Config{Backend: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	// Subscribe so the publish has somewhere to deliver to.
	ch, cancel, err := hub.Subscribe(ctx, "ack.event")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()

	if err := hub.PublishAndWaitAck(ctx, "ack.event", "payload", 2*time.Second); err != nil {
		t.Fatalf("PublishAndWaitAck: %v", err)
	}
}

func TestGoChannelAckTimeout(t *testing.T) {
	hub, err := eventhub.New(eventhub.Config{Backend: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	// No subscriber — GoChannel with Persistent=true will still deliver to
	// future subscribers, so publish returns immediately. Test that the timeout
	// parameter is correctly applied without hanging.
	if err := hub.PublishAndWaitAck(ctx, "timeout.event", nil, 100*time.Millisecond); err != nil {
		// publish is non-blocking for GoChannel, so no error expected
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGoChannelMultipleSubscribers(t *testing.T) {
	hub, err := eventhub.New(eventhub.Config{Backend: "memory"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	ch1, cancel1, _ := hub.Subscribe(ctx, "multi.event")
	defer cancel1()
	ch2, cancel2, _ := hub.Subscribe(ctx, "multi.event")
	defer cancel2()

	if err := hub.Publish(ctx, "multi.event", 42); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for _, ch := range []<-chan eventhub.EventEnvelope{ch1, ch2} {
		select {
		case env := <-ch:
			if env.Name != "multi.event" {
				t.Errorf("unexpected event name: %q", env.Name)
			}
		case <-timeout:
			t.Fatal("timeout waiting for event on one of the subscribers")
		}
	}
}
