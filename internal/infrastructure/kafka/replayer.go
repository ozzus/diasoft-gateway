package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/config"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ReplayPublisher interface {
	Publish(ctx context.Context, message DLQMessage) error
}

type kafkaReplayPublisher struct {
	client *kgo.Client
}

type ReplayPolicy struct {
	sourceTopics  map[string]struct{}
	eventTypes    map[string]struct{}
	failureStages map[string]struct{}
	eventIDs      map[string]struct{}
	maxMessages   int64
	dryRun        bool
}

type Replayer struct {
	client         *kgo.Client
	logger         *slog.Logger
	publisher      ReplayPublisher
	metrics        *appmetrics.Service
	policy         ReplayPolicy
	handledMatches int64
}

func NewReplayer(ctx context.Context, logger *slog.Logger, cfg config.KafkaConfig, metrics *appmetrics.Service) (*Replayer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	if strings.TrimSpace(cfg.DLQTopic) == "" {
		return nil, fmt.Errorf("kafka dlq topic is required")
	}
	if strings.TrimSpace(cfg.ReplayConsumerGroup) == "" {
		return nil, fmt.Errorf("kafka replay consumer group is required")
	}

	opts, err := baseClientOptions(cfg)
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		kgo.ConsumerGroup(cfg.ReplayConsumerGroup),
		kgo.ConsumeTopics(cfg.DLQTopic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create kafka replay client: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping kafka cluster: %w", err)
	}

	return &Replayer{
		client:    client,
		logger:    logger,
		publisher: NewReplayPublisher(client),
		metrics:   metrics,
		policy:    NewReplayPolicy(cfg),
	}, nil
}

func NewReplayPolicy(cfg config.KafkaConfig) ReplayPolicy {
	return ReplayPolicy{
		sourceTopics:  stringSet(cfg.ReplaySourceTopics),
		eventTypes:    stringSet(cfg.ReplayEventTypes),
		failureStages: stringSet(cfg.ReplayFailureStages),
		eventIDs:      stringSet(cfg.ReplayEventIDs),
		maxMessages:   cfg.ReplayMaxMessages,
		dryRun:        cfg.ReplayDryRun,
	}
}

func (p ReplayPolicy) Matches(message DLQMessage) bool {
	if len(p.sourceTopics) > 0 {
		if _, ok := p.sourceTopics[strings.TrimSpace(message.SourceTopic)]; !ok {
			return false
		}
	}
	if len(p.eventTypes) > 0 {
		if _, ok := p.eventTypes[strings.TrimSpace(message.EventType)]; !ok {
			return false
		}
	}
	if len(p.failureStages) > 0 {
		if _, ok := p.failureStages[strings.TrimSpace(message.FailureStage)]; !ok {
			return false
		}
	}
	if len(p.eventIDs) > 0 {
		if _, ok := p.eventIDs[strings.TrimSpace(message.EventID)]; !ok {
			return false
		}
	}
	return true
}

func (p ReplayPolicy) DryRun() bool {
	return p.dryRun
}

func (p ReplayPolicy) MaxMessages() int64 {
	return p.maxMessages
}

func NewReplayPublisher(client *kgo.Client) ReplayPublisher {
	if client == nil {
		return nil
	}
	return &kafkaReplayPublisher{client: client}
}

func (p *kafkaReplayPublisher) Publish(ctx context.Context, message DLQMessage) error {
	record := &kgo.Record{
		Topic:   strings.TrimSpace(message.SourceTopic),
		Key:     message.ReplayKey(),
		Value:   append([]byte(nil), message.SourceValue...),
		Headers: injectTraceHeaders(ctx, message.ReplayHeaders()),
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce replayed dlq message: %w", err)
	}
	return nil
}

func (r *Replayer) Run(ctx context.Context) error {
	defer r.client.Close()

	for {
		fetches := r.client.PollFetches(ctx)
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
		var stopAfterCommit bool
		fetches.EachRecord(func(record *kgo.Record) {
			if processErr != nil || stopAfterCommit {
				return
			}
			stop, err := r.processRecord(ctx, record)
			if err != nil {
				processErr = err
				return
			}
			stopAfterCommit = stop
		})
		if processErr != nil {
			return processErr
		}

		if err := r.client.CommitUncommittedOffsets(ctx); err != nil {
			return fmt.Errorf("commit replay offsets: %w", err)
		}
		if stopAfterCommit {
			r.logger.Info("stopping dlq replayer after reaching replay limit", "handled_matches", r.handledMatches, "replay_max_messages", r.policy.MaxMessages())
			return nil
		}
	}
}

func (r *Replayer) Ping(ctx context.Context) error {
	return r.client.Ping(ctx)
}

func (r *Replayer) processRecord(ctx context.Context, record *kgo.Record) (bool, error) {
	ctx = extractTraceContext(ctx, record.Headers)
	ctx, span := kafkaTracer.Start(ctx, "kafka.dlq_replayer.process_record",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(kafkaRecordAttributes(record)...),
	)
	defer span.End()

	message, err := ParseDLQMessage(record.Value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		if r.metrics != nil {
			r.metrics.ObserveKafkaEvent(record.Topic, "unknown", "replay_skipped")
		}
		r.logger.Warn("skipping malformed dlq message", "dlq_topic", record.Topic, "partition", record.Partition, "offset", record.Offset, "error", err)
		return false, nil
	}
	span.SetAttributes(
		attribute.String("app.kafka.source_topic", message.SourceTopic),
		attribute.String("app.kafka.event_id", message.EventID),
		attribute.String("app.kafka.event_type", message.EventType),
		attribute.String("app.kafka.failure_stage", message.FailureStage),
	)

	if !r.policy.Matches(message) {
		span.SetAttributes(attribute.String("app.kafka.replay_decision", "filtered"))
		span.SetStatus(codes.Ok, "filtered")
		if r.metrics != nil {
			r.metrics.ObserveKafkaEvent(message.SourceTopic, message.EventType, "replay_filtered")
		}
		r.logger.Debug("filtered dlq message from replay", "source_topic", message.SourceTopic, "event_id", message.EventID, "event_type", message.EventType, "failure_stage", message.FailureStage)
		return false, nil
	}

	if r.publisher == nil {
		err := fmt.Errorf("replay publisher is not configured")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}

	if r.policy.DryRun() {
		span.SetAttributes(attribute.String("app.kafka.replay_decision", "dry_run"))
		span.SetStatus(codes.Ok, "dry_run")
		r.handledMatches++
		if r.metrics != nil {
			r.metrics.ObserveKafkaEvent(message.SourceTopic, message.EventType, "replay_dry_run")
		}
		r.logger.Info("dry-run matched dlq message", "source_topic", message.SourceTopic, "event_id", message.EventID, "event_type", message.EventType, "failure_stage", message.FailureStage, "handled_matches", r.handledMatches)
		return r.shouldStop(), nil
	}
	if err := r.publisher.Publish(ctx, message); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		if r.metrics != nil {
			r.metrics.ObserveKafkaEvent(message.SourceTopic, message.EventType, "replay_failed")
		}
		return false, fmt.Errorf("replay dlq message topic=%s partition=%d offset=%d to source_topic=%s: %w", record.Topic, record.Partition, record.Offset, message.SourceTopic, err)
	}

	span.SetAttributes(attribute.String("app.kafka.replay_decision", "replayed"))
	span.SetStatus(codes.Ok, "replayed")
	r.handledMatches++
	if r.metrics != nil {
		r.metrics.ObserveKafkaEvent(message.SourceTopic, message.EventType, "replayed")
	}
	r.logger.Info("replayed dlq message", "dlq_topic", record.Topic, "partition", record.Partition, "offset", record.Offset, "source_topic", message.SourceTopic, "event_id", message.EventID, "event_type", message.EventType, "handled_matches", r.handledMatches)
	return r.shouldStop(), nil
}

func (r *Replayer) shouldStop() bool {
	return r.policy.MaxMessages() > 0 && r.handledMatches >= r.policy.MaxMessages()
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
