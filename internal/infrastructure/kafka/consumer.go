package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	"github.com/ssovich/diasoft-gateway/internal/config"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Consumer struct {
	client                 *kgo.Client
	logger                 *slog.Logger
	projector              EventProjector
	dlqWriter              DLQWriter
	metrics                *appmetrics.Service
	projectRetryAttempts   int
	projectRetryBackoff    time.Duration
	projectRetryMaxBackoff time.Duration
}

func NewConsumer(ctx context.Context, logger *slog.Logger, cfg config.KafkaConfig, projector EventProjector, metrics *appmetrics.Service) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}

	opts, err := baseClientOptions(cfg)
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.DiplomaTopic, cfg.ShareLinkTopic),
		kgo.DisableAutoCommit(),
	)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}

	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping kafka cluster: %w", err)
	}

	return &Consumer{
		client:                 client,
		logger:                 logger,
		projector:              projector,
		dlqWriter:              NewDLQWriter(client, cfg.DLQTopic),
		metrics:                metrics,
		projectRetryAttempts:   cfg.ProjectRetryAttempts,
		projectRetryBackoff:    cfg.ProjectRetryBackoff,
		projectRetryMaxBackoff: cfg.ProjectRetryMaxBackoff,
	}, nil
}

func (c *Consumer) Run(ctx context.Context) error {
	defer c.client.Close()

	for {
		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			return fmt.Errorf("kafka fetch errors: %v", errs)
		}

		var processErr error
		fetches.EachRecord(func(record *kgo.Record) {
			if processErr != nil {
				return
			}
			if err := c.processRecord(ctx, record); err != nil {
				processErr = err
			}
		})
		if processErr != nil {
			return processErr
		}

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			return fmt.Errorf("commit kafka offsets: %w", err)
		}
	}
}

func (c *Consumer) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}

func (c *Consumer) processRecord(ctx context.Context, record *kgo.Record) error {
	ctx = extractTraceContext(ctx, record.Headers)
	ctx, span := kafkaTracer.Start(ctx, "kafka.consumer.process_record",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(kafkaRecordAttributes(record)...),
	)
	defer span.End()

	var event usecase.EventEnvelope
	if err := json.Unmarshal(record.Value, &event); err != nil {
		return c.handleFailure(ctx, record, nil, "decode", err)
	}
	span.SetAttributes(kafkaEventAttributes(event)...)

	if err := c.projectWithRetry(ctx, record, event); err != nil {
		if ctx.Err() != nil {
			return err
		}
		return c.handleFailure(ctx, record, &event, "project", err)
	}

	if c.metrics != nil {
		c.metrics.ObserveKafkaEvent(record.Topic, event.EventType, "processed")
		if !event.OccurredAt.IsZero() {
			c.metrics.ObserveKafkaEventAge(record.Topic, event.EventType, time.Since(event.OccurredAt.UTC()))
		}
	}
	span.SetStatus(codes.Ok, "processed")
	c.logger.Info("processed kafka event", "topic", record.Topic, "partition", record.Partition, "offset", record.Offset, "event_id", event.EventID, "event_type", event.EventType)
	return nil
}

func (c *Consumer) projectWithRetry(ctx context.Context, record *kgo.Record, event usecase.EventEnvelope) error {
	err := c.projector.Handle(ctx, event)
	if err == nil {
		return nil
	}

	span := trace.SpanFromContext(ctx)
	for attempt := 1; attempt <= c.projectRetryAttempts; attempt++ {
		backoff := c.retryBackoff(attempt)
		if c.metrics != nil {
			c.metrics.ObserveKafkaEvent(record.Topic, event.EventType, "retry")
		}
		if span.IsRecording() {
			span.AddEvent("kafka.project.retry",
				trace.WithAttributes(
					attribute.Int("app.kafka.retry_attempt", attempt),
					attribute.String("app.kafka.retry_backoff", backoff.String()),
					attribute.String("app.kafka.retry_error", err.Error()),
				),
			)
		}
		c.logger.Warn("projection failed, retrying kafka event",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"event_id", event.EventID,
			"event_type", event.EventType,
			"retry_attempt", attempt,
			"retry_backoff", backoff,
			"error", err,
		)

		if waitErr := sleepContext(ctx, backoff); waitErr != nil {
			return waitErr
		}

		err = c.projector.Handle(ctx, event)
		if err == nil {
			if span.IsRecording() {
				span.SetAttributes(attribute.Int("app.kafka.retry_attempts", attempt))
			}
			if c.metrics != nil {
				c.metrics.ObserveKafkaEvent(record.Topic, event.EventType, "processed_after_retry")
			}
			return nil
		}
	}

	return err
}

func (c *Consumer) handleFailure(ctx context.Context, record *kgo.Record, event *usecase.EventEnvelope, stage string, cause error) error {
	eventType := "unknown"
	eventID := ""
	if event != nil {
		eventType = event.EventType
		eventID = event.EventID
	}
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("messaging.operation", "process"),
			attribute.String("messaging.destination.name", record.Topic),
			attribute.String("app.kafka.failure_stage", stage),
			attribute.String("app.kafka.event_id", eventID),
			attribute.String("app.kafka.event_type", eventType),
		)
		span.RecordError(cause)
		span.SetStatus(codes.Error, cause.Error())
	}
	if c.metrics != nil {
		c.metrics.ObserveKafkaEvent(record.Topic, eventType, "failed")
	}
	if c.dlqWriter == nil {
		return fmt.Errorf("%s event topic=%s partition=%d offset=%d: %w", stage, record.Topic, record.Partition, record.Offset, cause)
	}

	message := NewDLQMessage(record, stage, cause, event)
	if err := c.dlqWriter.Write(ctx, message); err != nil {
		if c.metrics != nil {
			c.metrics.ObserveKafkaEvent(record.Topic, eventType, "dlq_failed")
		}
		return fmt.Errorf("write dlq message topic=%s partition=%d offset=%d: %w", record.Topic, record.Partition, record.Offset, err)
	}

	if c.metrics != nil {
		c.metrics.ObserveKafkaEvent(record.Topic, eventType, "dlq_published")
	}
	c.logger.Warn("published kafka record to dlq", "source_topic", record.Topic, "source_partition", record.Partition, "source_offset", record.Offset, "event_id", eventID, "event_type", eventType, "failure_stage", stage, "error", cause)
	return nil
}

func kafkaRecordAttributes(record *kgo.Record) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", record.Topic),
		attribute.Int64("messaging.kafka.partition", int64(record.Partition)),
		attribute.Int64("messaging.kafka.offset", record.Offset),
	}
}

func kafkaEventAttributes(event usecase.EventEnvelope) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("app.kafka.event_id", event.EventID),
		attribute.String("app.kafka.event_type", event.EventType),
	}
	if !event.OccurredAt.IsZero() {
		attrs = append(attrs, attribute.String("app.kafka.occurred_at", event.OccurredAt.UTC().Format(time.RFC3339Nano)))
	}
	return attrs
}

func (c *Consumer) retryBackoff(attempt int) time.Duration {
	if attempt <= 0 || c.projectRetryBackoff <= 0 {
		return 0
	}
	backoff := c.projectRetryBackoff
	for idx := 1; idx < attempt; idx++ {
		if c.projectRetryMaxBackoff > 0 && backoff >= c.projectRetryMaxBackoff {
			return c.projectRetryMaxBackoff
		}
		backoff *= 2
		if c.projectRetryMaxBackoff > 0 && backoff > c.projectRetryMaxBackoff {
			return c.projectRetryMaxBackoff
		}
	}
	return backoff
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
