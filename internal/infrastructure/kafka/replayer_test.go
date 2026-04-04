package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type replayPublisherStub struct {
	messages []DLQMessage
	err      error
}

func (s *replayPublisherStub) Publish(_ context.Context, message DLQMessage) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

func TestParseDLQMessageValidatesRequiredFields(t *testing.T) {
	_, err := ParseDLQMessage([]byte(`{"event_id":"evt-1"}`))
	if err == nil {
		t.Fatal("ParseDLQMessage() error = nil, want non-nil")
	}
}

func TestDLQMessageReplayHeaders(t *testing.T) {
	message := DLQMessage{
		FailureStage:    "project",
		SourceTopic:     "diploma.lifecycle.v1",
		SourcePartition: 2,
		SourceOffset:    7,
		EventID:         "evt-1",
		EventType:       "diploma.updated.v1",
	}

	headers := message.ReplayHeaders()
	if len(headers) < 6 {
		t.Fatalf("headers count = %d, want at least 6", len(headers))
	}
	if headers[0].Key != "x-dlq-replayed" || string(headers[0].Value) != "true" {
		t.Fatalf("unexpected first replay header: %+v", headers[0])
	}
}

func TestReplayPolicyMatchesFilters(t *testing.T) {
	policy := ReplayPolicy{
		sourceTopics:  map[string]struct{}{"diploma.lifecycle.v1": {}},
		eventTypes:    map[string]struct{}{"diploma.updated.v1": {}},
		failureStages: map[string]struct{}{"project": {}},
		eventIDs:      map[string]struct{}{"evt-1": {}},
	}

	if !policy.Matches(DLQMessage{
		SourceTopic:  "diploma.lifecycle.v1",
		EventType:    "diploma.updated.v1",
		FailureStage: "project",
		EventID:      "evt-1",
	}) {
		t.Fatal("policy should match message")
	}

	if policy.Matches(DLQMessage{
		SourceTopic:  "sharelink.lifecycle.v1",
		EventType:    "diploma.updated.v1",
		FailureStage: "project",
		EventID:      "evt-1",
	}) {
		t.Fatal("policy should reject message with different source topic")
	}
}

func TestReplayerProcessRecordPublishesReplay(t *testing.T) {
	publisher := &replayPublisherStub{}
	replayer := &Replayer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		publisher: publisher,
	}

	payload, err := json.Marshal(DLQMessage{
		SourceTopic:  "diploma.lifecycle.v1",
		SourceKey:    []byte("key-1"),
		SourceValue:  []byte(`{"event_id":"evt-1"}`),
		EventID:      "evt-1",
		EventType:    "diploma.updated.v1",
		FailedAt:     time.Now().UTC(),
		FailureStage: "project",
	})
	if err != nil {
		t.Fatalf("marshal dlq message: %v", err)
	}

	stop, err := replayer.processRecord(context.Background(), &kgo.Record{Topic: "gateway.dlq.v1", Partition: 0, Offset: 1, Value: payload})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if stop {
		t.Fatal("processRecord() stop = true, want false")
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages = %d, want 1", len(publisher.messages))
	}
	if publisher.messages[0].SourceTopic != "diploma.lifecycle.v1" {
		t.Fatalf("source topic = %s, want diploma.lifecycle.v1", publisher.messages[0].SourceTopic)
	}
}

func TestReplayerProcessRecordSkipsMalformedPayload(t *testing.T) {
	replayer := &Replayer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		publisher: &replayPublisherStub{},
	}

	stop, err := replayer.processRecord(context.Background(), &kgo.Record{Topic: "gateway.dlq.v1", Partition: 0, Offset: 2, Value: []byte("broken")})
	if err != nil {
		t.Fatalf("processRecord() error = %v, want nil", err)
	}
	if stop {
		t.Fatal("processRecord() stop = true, want false")
	}
}

func TestReplayerProcessRecordReturnsErrorWhenPublishFails(t *testing.T) {
	replayer := &Replayer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		publisher: &replayPublisherStub{err: errors.New("producer unavailable")},
	}

	message := DLQMessage{
		SourceTopic:  "diploma.lifecycle.v1",
		SourceValue:  []byte(`{"event_id":"evt-1"}`),
		EventID:      "evt-1",
		EventType:    "diploma.updated.v1",
		FailureStage: "project",
		FailedAt:     time.Now().UTC(),
	}
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal dlq message: %v", err)
	}

	_, err = replayer.processRecord(context.Background(), &kgo.Record{Topic: "gateway.dlq.v1", Partition: 1, Offset: 3, Value: payload})
	if err == nil {
		t.Fatal("processRecord() error = nil, want non-nil")
	}
}

func TestReplayerProcessRecordFiltersNonMatchingMessage(t *testing.T) {
	publisher := &replayPublisherStub{}
	replayer := &Replayer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		publisher: publisher,
		policy: ReplayPolicy{
			sourceTopics: map[string]struct{}{"diploma.lifecycle.v1": {}},
		},
	}

	payload, err := json.Marshal(DLQMessage{
		SourceTopic:  "sharelink.lifecycle.v1",
		SourceValue:  []byte(`{"event_id":"evt-1"}`),
		EventID:      "evt-1",
		EventType:    "sharelink.created.v1",
		FailureStage: "project",
		FailedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal dlq message: %v", err)
	}

	stop, err := replayer.processRecord(context.Background(), &kgo.Record{Topic: "gateway.dlq.v1", Partition: 0, Offset: 4, Value: payload})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if stop {
		t.Fatal("processRecord() stop = true, want false")
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0", len(publisher.messages))
	}
}

func TestReplayerProcessRecordDryRunStopsAfterLimit(t *testing.T) {
	publisher := &replayPublisherStub{}
	replayer := &Replayer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		publisher: publisher,
		policy: ReplayPolicy{
			maxMessages: 1,
			dryRun:      true,
		},
	}

	payload, err := json.Marshal(DLQMessage{
		SourceTopic:  "diploma.lifecycle.v1",
		SourceValue:  []byte(`{"event_id":"evt-1"}`),
		EventID:      "evt-1",
		EventType:    "diploma.updated.v1",
		FailureStage: "project",
		FailedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal dlq message: %v", err)
	}

	stop, err := replayer.processRecord(context.Background(), &kgo.Record{Topic: "gateway.dlq.v1", Partition: 0, Offset: 5, Value: payload})
	if err != nil {
		t.Fatalf("processRecord() error = %v", err)
	}
	if !stop {
		t.Fatal("processRecord() stop = false, want true")
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0 in dry-run", len(publisher.messages))
	}
}

func TestDLQMessageReplayKeyFallsBackToEventID(t *testing.T) {
	message := DLQMessage{EventID: "evt-1"}

	if string(message.ReplayKey()) != "evt-1" {
		t.Fatalf("ReplayKey() = %s, want evt-1", string(message.ReplayKey()))
	}
}

func TestDLQMessageReplayKeyReturnsSourceKeyWhenPresent(t *testing.T) {
	message := DLQMessage{SourceKey: []byte("source-key"), EventID: "evt-1"}
	if string(message.ReplayKey()) != "source-key" {
		t.Fatalf("ReplayKey() = %s, want source-key", string(message.ReplayKey()))
	}
}

func TestInjectTraceHeadersWithoutSpanDoesNotAddHeaders(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	headers := injectTraceHeaders(context.Background(), nil)
	if len(headers) != 0 {
		t.Fatalf("headers count = %d, want 0", len(headers))
	}
}

func TestKafkaHeaderCarrierRoundTrip(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	headers := []kgo.RecordHeader{{Key: "existing", Value: []byte("value")}}
	carrier := kafkaHeaderCarrier{headers: &headers}
	carrier.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	if got := carrier.Get("traceparent"); got == "" {
		t.Fatal("traceparent header was not stored")
	}
	if len(carrier.Keys()) != 2 {
		t.Fatalf("keys = %d, want 2", len(carrier.Keys()))
	}

	ctx := extractTraceContext(context.Background(), headers)
	if spanCtx := trace.SpanContextFromContext(ctx); !spanCtx.IsValid() {
		t.Fatal("extracted span context is not valid")
	}
}
