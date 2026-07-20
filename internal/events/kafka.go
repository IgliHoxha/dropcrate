package events

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/segmentio/kafka-go"
)

// Kafka is a best-effort Publisher backed by an async kafka-go writer. Messages
// are queued and delivered in the background; delivery failures are logged, not
// surfaced to the caller. One event type maps to one topic: prefix + Event.Type.
type Kafka struct {
	writer *kafka.Writer
	prefix string
	log    *slog.Logger
}

// NewKafka builds a Kafka publisher writing to the given brokers. topicPrefix is
// prepended to each event type to form the topic name (e.g. "dropcrate." +
// "file.uploaded"). The writer is asynchronous, so Publish never blocks on the
// broker.
func NewKafka(brokers []string, topicPrefix string, log *slog.Logger) *Kafka {
	k := &Kafka{prefix: topicPrefix, log: log}
	k.writer = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{},
		Async:                  true,
		AllowAutoTopicCreation: true,
		Completion: func(_ []kafka.Message, err error) {
			if err != nil {
				log.Error("kafka delivery failed", "error", err)
			}
		},
	}
	return k
}

// Publish JSON-encodes the event and queues it for the topic prefix+Type, keyed
// by file id so all events for one file land on the same partition.
func (k *Kafka) Publish(ctx context.Context, e Event) {
	payload, err := json.Marshal(e)
	if err != nil {
		k.log.Error("kafka marshal event", "error", err)
		return
	}
	// With an async writer this returns immediately; real delivery errors are
	// reported through the Completion callback above.
	if err := k.writer.WriteMessages(ctx, kafka.Message{
		Topic: k.prefix + e.Type,
		Key:   []byte(e.FileID),
		Value: payload,
	}); err != nil {
		k.log.Error("kafka enqueue event", "error", err)
	}
}

// Close flushes buffered messages and closes the writer.
func (k *Kafka) Close() error {
	return k.writer.Close()
}
