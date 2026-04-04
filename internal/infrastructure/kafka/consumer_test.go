package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	"github.com/twmb/franz-go/pkg/kgo"
)

type projectorStub struct {
	err   error
	errs  []error
	calls int
}

func (s *projectorStub) Handle(context.Context, usecase.EventEnvelope) error {
	s.calls++
	if len(s.errs) > 0 {
		idx := s.calls - 1
		if idx < len(s.errs) {
			return s.errs[idx]
		}
		return nil
	}
	return s.err
}

type dlqWriterStub struct {
	messages []DLQMessage
	err      error
}

func (s *dlqWriterStub) Write(_ context.Context, message DLQMessage) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

func TestConsumerProcessRecordPublishesDecodeFailureToDLQ(t *testing.T) {
	writer := &dlqWriterStub{}
	consumer := &Consumer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		projector: &projectorStub{},
		dlqWriter: writer,
	}

	err := consumer.processRecord(context.Background(), &kgo.Record{Topic: "diploma.lifecycle.v1", Partition: 1, Offset: 2, Value: []byte("not-json")})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if len(writer.messages) != 1 {
		t.Fatalf("dlq messages = %d, want 1", len(writer.messages))
	}
	if writer.messages[0].FailureStage != "decode" {
		t.Fatalf("failure stage = %s, want decode", writer.messages[0].FailureStage)
	}
}

func TestConsumerProcessRecordPublishesProjectFailureToDLQ(t *testing.T) {
	writer := &dlqWriterStub{}
	projector := &projectorStub{err: errors.New("projection failed")}
	consumer := &Consumer{
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		projector:            projector,
		dlqWriter:            writer,
		projectRetryAttempts: 2,
	}

	payload := []byte(`{"event_id":"evt-1","event_type":"diploma.updated.v1","occurred_at":"2026-04-03T10:00:00Z","payload":{}}`)
	err := consumer.processRecord(context.Background(), &kgo.Record{Topic: "diploma.lifecycle.v1", Partition: 2, Offset: 5, Value: payload})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if len(writer.messages) != 1 {
		t.Fatalf("dlq messages = %d, want 1", len(writer.messages))
	}
	if writer.messages[0].FailureStage != "project" {
		t.Fatalf("failure stage = %s, want project", writer.messages[0].FailureStage)
	}
	if writer.messages[0].EventID != "evt-1" {
		t.Fatalf("event id = %s, want evt-1", writer.messages[0].EventID)
	}
	if projector.calls != 3 {
		t.Fatalf("projector calls = %d, want 3", projector.calls)
	}
}

func TestConsumerProcessRecordReturnsErrorWhenDLQPublishFails(t *testing.T) {
	consumer := &Consumer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		projector: &projectorStub{},
		dlqWriter: &dlqWriterStub{err: errors.New("dlq unavailable")},
	}

	err := consumer.processRecord(context.Background(), &kgo.Record{Topic: "sharelink.lifecycle.v1", Partition: 0, Offset: 7, Value: []byte("broken")})
	if err == nil {
		t.Fatal("processRecord() error = nil, want non-nil")
	}
}

func TestConsumerProcessRecordRetriesProjectionAndSucceeds(t *testing.T) {
	writer := &dlqWriterStub{}
	projector := &projectorStub{errs: []error{errors.New("temporary projection failure")}}
	consumer := &Consumer{
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		projector:            projector,
		dlqWriter:            writer,
		projectRetryAttempts: 2,
	}

	payload := []byte(`{"event_id":"evt-1","event_type":"diploma.updated.v1","occurred_at":"2026-04-03T10:00:00Z","payload":{}}`)
	err := consumer.processRecord(context.Background(), &kgo.Record{Topic: "diploma.lifecycle.v1", Partition: 2, Offset: 5, Value: payload})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("dlq messages = %d, want 0", len(writer.messages))
	}
	if projector.calls != 2 {
		t.Fatalf("projector calls = %d, want 2", projector.calls)
	}
}

func TestConsumerRetryBackoffCapsAtMax(t *testing.T) {
	consumer := &Consumer{
		projectRetryBackoff:    250 * time.Millisecond,
		projectRetryMaxBackoff: 1 * time.Second,
	}

	if got := consumer.retryBackoff(1); got != 250*time.Millisecond {
		t.Fatalf("retryBackoff(1) = %s, want 250ms", got)
	}
	if got := consumer.retryBackoff(2); got != 500*time.Millisecond {
		t.Fatalf("retryBackoff(2) = %s, want 500ms", got)
	}
	if got := consumer.retryBackoff(3); got != 1*time.Second {
		t.Fatalf("retryBackoff(3) = %s, want 1s", got)
	}
	if got := consumer.retryBackoff(4); got != 1*time.Second {
		t.Fatalf("retryBackoff(4) = %s, want 1s", got)
	}
}

func TestNewDLQMessageCopiesRecordFields(t *testing.T) {
	event := &usecase.EventEnvelope{EventID: "evt-2", EventType: "sharelink.created.v1", OccurredAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)}
	record := &kgo.Record{Topic: "sharelink.lifecycle.v1", Partition: 3, Offset: 11, Key: []byte("key"), Value: []byte("value")}

	message := NewDLQMessage(record, "project", errors.New("boom"), event)
	if message.SourceTopic != record.Topic {
		t.Fatalf("source topic = %s, want %s", message.SourceTopic, record.Topic)
	}
	if message.EventType != event.EventType {
		t.Fatalf("event type = %s, want %s", message.EventType, event.EventType)
	}
	if string(message.SourceKey) != "key" || string(message.SourceValue) != "value" {
		t.Fatalf("record payload was not copied")
	}
}
