package eventhub

// EventEnvelope is the wire format for events sent over the pub/sub layer.
// It is JSON-marshaled into the Watermill message payload.
type EventEnvelope struct {
	Name    string `json:"name"`
	Payload any    `json:"payload"`
}
