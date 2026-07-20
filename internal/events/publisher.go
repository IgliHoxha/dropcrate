package events

import "context"

// Publisher emits domain events. Implementations must be best-effort: Publish
// never blocks the caller on the broker and never returns a transport error,
// because a messaging problem must not fail a file operation.
type Publisher interface {
	// Publish emits one event. It returns quickly; delivery is asynchronous.
	Publish(ctx context.Context, e Event)
	// Close flushes and releases any resources.
	Close() error
}

// Nop is the default Publisher used when no broker is configured. Every method
// does nothing, so the rest of the app is oblivious to whether events are wired.
type Nop struct{}

// Publish discards the event.
func (Nop) Publish(context.Context, Event) {}

// Close is a no-op.
func (Nop) Close() error { return nil }
