package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	"github.com/twmb/franz-go/pkg/kgo"
)

type EventProjector interface {
	Handle(ctx context.Context, event usecase.EventEnvelope) error
}

type DLQWriter interface {
	Write(ctx context.Context, message DLQMessage) error
}

type DLQMessage struct {
	FailedAt        time.Time `json:"failed_at"`
	FailureStage    string    `json:"failure_stage"`
	Error           string    `json:"error"`
	SourceTopic     string    `json:"source_topic"`
	SourcePartition int32     `json:"source_partition"`
	SourceOffset    int64     `json:"source_offset"`
	SourceKey       []byte    `json:"source_key,omitempty"`
	SourceValue     []byte    `json:"source_value"`
	EventID         string    `json:"event_id,omitempty"`
	EventType       string    `json:"event_type,omitempty"`
	OccurredAt      time.Time `json:"occurred_at,omitempty"`
}

func ParseDLQMessage(raw []byte) (DLQMessage, error) {
	var message DLQMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return DLQMessage{}, fmt.Errorf("decode dlq message: %w", err)
	}
	if err := message.Validate(); err != nil {
		return DLQMessage{}, err
	}
	return message, nil
}

func (m DLQMessage) Validate() error {
	if strings.TrimSpace(m.SourceTopic) == "" {
		return fmt.Errorf("dlq source_topic is required")
	}
	return nil
}

func (m DLQMessage) ReplayKey() []byte {
	if len(m.SourceKey) > 0 {
		return append([]byte(nil), m.SourceKey...)
	}
	if strings.TrimSpace(m.EventID) == "" {
		return nil
	}
	return []byte(strings.TrimSpace(m.EventID))
}

func (m DLQMessage) ReplayHeaders() []kgo.RecordHeader {
	headers := []kgo.RecordHeader{
		{Key: "x-dlq-replayed", Value: []byte("true")},
		{Key: "x-dlq-failure-stage", Value: []byte(strings.TrimSpace(m.FailureStage))},
		{Key: "x-dlq-original-topic", Value: []byte(strings.TrimSpace(m.SourceTopic))},
		{Key: "x-dlq-original-partition", Value: []byte(strconv.FormatInt(int64(m.SourcePartition), 10))},
		{Key: "x-dlq-original-offset", Value: []byte(strconv.FormatInt(m.SourceOffset, 10))},
	}
	if strings.TrimSpace(m.EventID) != "" {
		headers = append(headers, kgo.RecordHeader{Key: "x-dlq-event-id", Value: []byte(strings.TrimSpace(m.EventID))})
	}
	if strings.TrimSpace(m.EventType) != "" {
		headers = append(headers, kgo.RecordHeader{Key: "x-dlq-event-type", Value: []byte(strings.TrimSpace(m.EventType))})
	}
	return headers
}

type kafkaDLQWriter struct {
	client *kgo.Client
	topic  string
}

func NewDLQWriter(client *kgo.Client, topic string) DLQWriter {
	if client == nil || strings.TrimSpace(topic) == "" {
		return nil
	}
	return &kafkaDLQWriter{client: client, topic: strings.TrimSpace(topic)}
}

func (w *kafkaDLQWriter) Write(ctx context.Context, message DLQMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal dlq message: %w", err)
	}

	key := []byte(message.EventID)
	if len(key) == 0 {
		key = message.SourceKey
	}

	if err := w.client.ProduceSync(ctx, &kgo.Record{
		Topic:   w.topic,
		Key:     key,
		Value:   payload,
		Headers: injectTraceHeaders(ctx, nil),
	}).FirstErr(); err != nil {
		return fmt.Errorf("produce dlq message: %w", err)
	}

	return nil
}

func NewDLQMessage(record *kgo.Record, stage string, cause error, event *usecase.EventEnvelope) DLQMessage {
	message := DLQMessage{
		FailedAt:        time.Now().UTC(),
		FailureStage:    strings.TrimSpace(stage),
		Error:           cause.Error(),
		SourceTopic:     record.Topic,
		SourcePartition: record.Partition,
		SourceOffset:    record.Offset,
		SourceKey:       append([]byte(nil), record.Key...),
		SourceValue:     append([]byte(nil), record.Value...),
	}
	if event != nil {
		message.EventID = strings.TrimSpace(event.EventID)
		message.EventType = strings.TrimSpace(event.EventType)
		message.OccurredAt = event.OccurredAt
	}
	return message
}
