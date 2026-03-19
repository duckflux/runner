package eventhub

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// newMemoryHub creates a Hub backed by an in-process GoChannel pub/sub.
// Persistent mode is enabled so subscribers created after a Publish still
// receive past events (replicating the previous in-process eventLog behaviour).
func newMemoryHub(cfg Config) (*Hub, error) {
	gc := gochannel.NewGoChannel(
		gochannel.Config{
			OutputChannelBuffer:            16,
			Persistent:                     true,
			BlockPublishUntilSubscriberAck: false,
		},
		watermill.NopLogger{},
	)

	return &Hub{
		publisher:  gc,
		subscriber: gc,
		config:     cfg,
		closers:    []func() error{gc.Close},
	}, nil
}
