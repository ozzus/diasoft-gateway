//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	"github.com/ssovich/diasoft-gateway/internal/config"
	domainsharelink "github.com/ssovich/diasoft-gateway/internal/domain/sharelink"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
	kafkainfra "github.com/ssovich/diasoft-gateway/internal/infrastructure/kafka"
	redisinfra "github.com/ssovich/diasoft-gateway/internal/infrastructure/redis"
	auditstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/audit"
	diplomastore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/diploma"
	readmodelstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/readmodel"
	sharelinkstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/sharelink"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	postgresImage = "postgres:16-alpine"
	redisImage    = "redis:7-alpine"
	redpandaImage = "docker.redpanda.com/redpandadata/redpanda:v25.2.4"

	diplomaTopic   = "diploma.lifecycle.v1"
	shareLinkTopic = "sharelink.lifecycle.v1"
	dlqTopic       = "gateway.dlq.v1"
)

type integrationEnv struct {
	dbPool      *pgxpool.Pool
	redisClient *goredis.Client
	kafkaBroker string
}

func TestVerifyUseCaseCachesDatabaseResult(t *testing.T) {
	ctx := context.Background()
	env := newIntegrationEnv(t)

	const (
		diplomaID         = "11111111-1111-1111-1111-111111111111"
		verificationToken = "token-cache-1"
		universityCode    = "MSU"
		diplomaNumber     = "D-100"
	)

	mustExec(t, env.dbPool, `insert into verification_records (diploma_id, verification_token, university_code, diploma_number, student_name_masked, program_name, status)
		values ($1::uuid, $2, $3, $4, $5, $6, $7)`, diplomaID, verificationToken, universityCode, diplomaNumber, "I*** I***", "Computer Science", string(domainverification.StatusValid))

	cache := redisinfra.NewVerificationCache(env.redisClient, time.Minute)
	verify := usecase.NewVerify(diplomastore.NewStore(env.dbPool), cache)

	result, err := verify.Run(ctx, usecase.VerifyCommand{DiplomaNumber: diplomaNumber, UniversityCode: universityCode})
	if err != nil {
		t.Fatalf("first verify run: %v", err)
	}
	if result.Verdict != domainverification.VerdictValid {
		t.Fatalf("first verify verdict = %s, want %s", result.Verdict, domainverification.VerdictValid)
	}

	mustExec(t, env.dbPool, `delete from verification_records where diploma_id = $1::uuid`, diplomaID)

	cached, err := verify.Run(ctx, usecase.VerifyCommand{DiplomaNumber: diplomaNumber, UniversityCode: universityCode})
	if err != nil {
		t.Fatalf("second verify run: %v", err)
	}
	if cached.Verdict != domainverification.VerdictValid {
		t.Fatalf("second verify verdict = %s, want %s", cached.Verdict, domainverification.VerdictValid)
	}
}

func TestResolveShareLinkExpiresAfterViewLimit(t *testing.T) {
	ctx := context.Background()
	env := newIntegrationEnv(t)

	const (
		diplomaID      = "22222222-2222-2222-2222-222222222222"
		shareToken     = "share-token-1"
		verificationID = "verification-token-2"
	)

	mustExec(t, env.dbPool, `insert into verification_records (diploma_id, verification_token, university_code, diploma_number, student_name_masked, program_name, status)
		values ($1::uuid, $2, $3, $4, $5, $6, $7)`, diplomaID, verificationID, "ITMO", "D-200", "P*** P***", "Data Science", string(domainverification.StatusValid))

	shareRepo := sharelinkstore.NewStore(env.dbPool)
	mustExec(t, env.dbPool, `insert into share_link_records (share_token, diploma_id, expires_at, max_views, used_views, status)
		values ($1, $2::uuid, $3, $4, $5, $6)`,
		shareToken,
		diplomaID,
		time.Now().UTC().Add(30*time.Minute),
		1,
		0,
		string(domainsharelink.StateActive),
	)

	resolver := usecase.NewResolveShareLink(shareRepo, diplomastore.NewStore(env.dbPool))

	first, err := resolver.Run(ctx, usecase.ResolveShareLinkCommand{Token: shareToken})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if first.Verdict != domainverification.VerdictValid {
		t.Fatalf("first resolve verdict = %s, want %s", first.Verdict, domainverification.VerdictValid)
	}

	var status string
	var usedViews int
	if err := env.dbPool.QueryRow(ctx, `select status, used_views from share_link_records where share_token = $1`, shareToken).Scan(&status, &usedViews); err != nil {
		t.Fatalf("query share link state: %v", err)
	}
	if status != string(domainsharelink.StateExpired) {
		t.Fatalf("share link status after first resolve = %s, want %s", status, domainsharelink.StateExpired)
	}
	if usedViews != 1 {
		t.Fatalf("share link used_views = %d, want 1", usedViews)
	}

	second, err := resolver.Run(ctx, usecase.ResolveShareLinkCommand{Token: shareToken})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if second.Verdict != domainverification.VerdictExpired {
		t.Fatalf("second resolve verdict = %s, want %s", second.Verdict, domainverification.VerdictExpired)
	}
}

func TestProjectEventsUpsertsReadModelAndInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	env := newIntegrationEnv(t)

	cache := redisinfra.NewVerificationCache(env.redisClient, time.Hour)
	lookupKey := usecase.VerificationLookupCacheKey("D-300", "bmstu")
	tokenKey := usecase.VerificationTokenCacheKey("verification-token-3")
	cachedResult := domainverification.Result{Verdict: domainverification.VerdictValid, UniversityCode: "BMSTU", DiplomaNumber: "D-300", OwnerNameMask: "S*** S***", Program: "Robotics"}

	if err := cache.Set(ctx, lookupKey, cachedResult); err != nil {
		t.Fatalf("cache set lookup key: %v", err)
	}
	if err := cache.Set(ctx, tokenKey, cachedResult); err != nil {
		t.Fatalf("cache set token key: %v", err)
	}

	payload, err := json.Marshal(usecase.DiplomaLifecyclePayload{
		DiplomaID:         "33333333-3333-3333-3333-333333333333",
		VerificationToken: "verification-token-3",
		UniversityCode:    "BMSTU",
		DiplomaNumber:     "D-300",
		StudentNameMasked: "S*** S***",
		ProgramName:       "Robotics",
		Status:            string(domainverification.StatusRevoked),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	projector := usecase.NewProjectEvents(readmodelstore.NewStore(env.dbPool), cache)
	if err := projector.Handle(ctx, usecase.EventEnvelope{EventID: "evt-1", EventType: "diploma.updated.v1", Payload: payload}); err != nil {
		t.Fatalf("project event: %v", err)
	}

	var persistedStatus string
	if err := env.dbPool.QueryRow(ctx, `select status from verification_records where diploma_id = $1::uuid`, "33333333-3333-3333-3333-333333333333").Scan(&persistedStatus); err != nil {
		t.Fatalf("query projected verification record: %v", err)
	}
	if persistedStatus != string(domainverification.StatusRevoked) {
		t.Fatalf("persisted status = %s, want %s", persistedStatus, domainverification.StatusRevoked)
	}

	if _, found, err := cache.Get(ctx, lookupKey); err != nil {
		t.Fatalf("cache get lookup key: %v", err)
	} else if found {
		t.Fatalf("lookup cache key should be invalidated")
	}
	if _, found, err := cache.Get(ctx, tokenKey); err != nil {
		t.Fatalf("cache get token key: %v", err)
	} else if found {
		t.Fatalf("token cache key should be invalidated")
	}

	var processed bool
	if err := env.dbPool.QueryRow(ctx, `select true from processed_events where event_id = $1`, "evt-1").Scan(&processed); err != nil {
		t.Fatalf("query processed event: %v", err)
	}
	if !processed {
		t.Fatalf("processed event marker was not written")
	}
}

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	ctx := context.Background()
	env := newIntegrationEnv(t)

	limiter := redisinfra.NewRateLimiter(env.redisClient, "itest", 1, time.Minute)

	allowed, err := limiter.Allow(ctx, "127.0.0.1:GET")
	if err != nil {
		t.Fatalf("first rate limit call: %v", err)
	}
	if !allowed {
		t.Fatalf("first request should be allowed")
	}

	allowed, err = limiter.Allow(ctx, "127.0.0.1:GET")
	if err != nil {
		t.Fatalf("second rate limit call: %v", err)
	}
	if allowed {
		t.Fatalf("second request should be blocked")
	}
}

func TestAuditStorePersistsPublicVerificationEvent(t *testing.T) {
	ctx := context.Background()
	env := newIntegrationEnv(t)

	auditUseCase := usecase.NewRecordVerificationAudit(auditstore.NewStore(env.dbPool))
	if err := auditUseCase.Run(ctx, usecase.RecordVerificationAuditCommand{
		RequestType:    "verify_lookup",
		Token:          "token-audit-1",
		DiplomaNumber:  "D-400",
		UniversityCode: "mifi",
		RemoteIP:       "10.0.0.1",
		Verdict:        domainverification.VerdictValid,
	}); err != nil {
		t.Fatalf("record audit event: %v", err)
	}

	var count int
	if err := env.dbPool.QueryRow(ctx, `select count(*) from verification_audit where request_type = $1 and verdict = $2 and university_code = $3`, "verify_lookup", string(domainverification.VerdictValid), "MIFI").Scan(&count); err != nil {
		t.Fatalf("query verification audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit row count = %d, want 1", count)
	}
}

func TestKafkaConsumerProjectsVerificationEvent(t *testing.T) {
	ctx := context.Background()
	env := newKafkaIntegrationEnv(t)

	cache := redisinfra.NewVerificationCache(env.redisClient, time.Hour)
	lookupKey := usecase.VerificationLookupCacheKey("D-500", "spbu")
	tokenKey := usecase.VerificationTokenCacheKey("verification-token-5")
	cached := domainverification.Result{
		Verdict:        domainverification.VerdictValid,
		DiplomaNumber:  "D-500",
		UniversityCode: "SPBU",
		OwnerNameMask:  "A*** A***",
		Program:        "Applied Mathematics",
	}
	if err := cache.Set(ctx, lookupKey, cached); err != nil {
		t.Fatalf("cache set lookup key: %v", err)
	}
	if err := cache.Set(ctx, tokenKey, cached); err != nil {
		t.Fatalf("cache set token key: %v", err)
	}

	projector := usecase.NewProjectEvents(readmodelstore.NewStore(env.dbPool), cache)
	consumerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := startKafkaConsumer(t, consumerCtx, env.kafkaBroker, uniqueConsumerGroup(t.Name()), projector)

	payload, err := json.Marshal(usecase.DiplomaLifecyclePayload{
		DiplomaID:         "55555555-5555-5555-5555-555555555555",
		VerificationToken: "verification-token-5",
		UniversityCode:    "SPBU",
		DiplomaNumber:     "D-500",
		StudentNameMasked: "A*** A***",
		ProgramName:       "Applied Mathematics",
		Status:            string(domainverification.StatusValid),
	})
	if err != nil {
		t.Fatalf("marshal diploma payload: %v", err)
	}

	produceEvent(t, env.kafkaBroker, diplomaTopic, usecase.EventEnvelope{
		EventID:      "evt-kafka-projection-1",
		EventType:    "diploma.updated.v1",
		EventVersion: "v1",
		OccurredAt:   time.Now().UTC(),
		Payload:      payload,
	})

	waitFor(t, 20*time.Second, func() error {
		var status string
		err := env.dbPool.QueryRow(ctx, `select status from verification_records where diploma_id = $1::uuid`, "55555555-5555-5555-5555-555555555555").Scan(&status)
		if err != nil {
			return err
		}
		if status != string(domainverification.StatusValid) {
			return fmt.Errorf("unexpected status %s", status)
		}
		return nil
	})

	waitFor(t, 10*time.Second, func() error {
		_, found, err := cache.Get(ctx, lookupKey)
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("lookup cache key was not invalidated")
		}
		return nil
	})

	waitFor(t, 10*time.Second, func() error {
		_, found, err := cache.Get(ctx, tokenKey)
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("token cache key was not invalidated")
		}
		return nil
	})

	var processed bool
	if err := env.dbPool.QueryRow(ctx, `select true from processed_events where event_id = $1`, "evt-kafka-projection-1").Scan(&processed); err != nil {
		t.Fatalf("query processed event: %v", err)
	}
	if !processed {
		t.Fatal("processed event marker was not written")
	}

	cancel()
	assertConsumerStopped(t, errCh)
}

func TestKafkaConsumerPublishesProjectionFailuresToDLQ(t *testing.T) {
	ctx := context.Background()
	env := newKafkaIntegrationEnv(t)

	projector := usecase.NewProjectEvents(readmodelstore.NewStore(env.dbPool), redisinfra.NewVerificationCache(env.redisClient, time.Minute))
	consumerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := startKafkaConsumer(t, consumerCtx, env.kafkaBroker, uniqueConsumerGroup(t.Name()), projector)

	dlqClient := newKafkaTopicConsumer(t, env.kafkaBroker, dlqTopic, kgo.NewOffset().AtEnd())
	defer dlqClient.Close()

	payload, err := json.Marshal(map[string]string{"unexpected": "payload"})
	if err != nil {
		t.Fatalf("marshal invalid payload: %v", err)
	}

	produceEvent(t, env.kafkaBroker, diplomaTopic, usecase.EventEnvelope{
		EventID:      "evt-kafka-dlq-1",
		EventType:    "unsupported.event.v1",
		EventVersion: "v1",
		OccurredAt:   time.Now().UTC(),
		Payload:      payload,
	})

	record := waitForKafkaRecord(t, dlqClient, 20*time.Second)

	var message kafkainfra.DLQMessage
	if err := json.Unmarshal(record.Value, &message); err != nil {
		t.Fatalf("unmarshal dlq message: %v", err)
	}
	if message.FailureStage != "project" {
		t.Fatalf("dlq failure stage = %s, want project", message.FailureStage)
	}
	if message.SourceTopic != diplomaTopic {
		t.Fatalf("dlq source topic = %s, want %s", message.SourceTopic, diplomaTopic)
	}
	if message.EventID != "evt-kafka-dlq-1" {
		t.Fatalf("dlq event id = %s, want evt-kafka-dlq-1", message.EventID)
	}
	if message.EventType != "unsupported.event.v1" {
		t.Fatalf("dlq event type = %s, want unsupported.event.v1", message.EventType)
	}

	var count int
	if err := env.dbPool.QueryRow(ctx, `select count(*) from processed_events where event_id = $1`, "evt-kafka-dlq-1").Scan(&count); err != nil {
		t.Fatalf("query processed events for dlq case: %v", err)
	}
	if count != 0 {
		t.Fatalf("processed event count = %d, want 0", count)
	}

	cancel()
	assertConsumerStopped(t, errCh)
}

func TestDLQReplayerReplaysMessageToSourceTopic(t *testing.T) {
	ctx := context.Background()
	env := newKafkaIntegrationEnv(t)

	replayerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := startDLQReplayer(t, replayerCtx, env.kafkaBroker, uniqueConsumerGroup(t.Name()))

	sourceConsumer := newKafkaTopicConsumer(t, env.kafkaBroker, diplomaTopic, kgo.NewOffset().AtEnd())
	defer sourceConsumer.Close()

	sourceValue := []byte(`{"event_id":"evt-replay-1","event_type":"diploma.updated.v1","payload":{}}`)
	dlqPayload, err := json.Marshal(kafkainfra.DLQMessage{
		FailedAt:        time.Now().UTC(),
		FailureStage:    "project",
		SourceTopic:     diplomaTopic,
		SourcePartition: 0,
		SourceOffset:    99,
		SourceKey:       []byte("evt-replay-1"),
		SourceValue:     sourceValue,
		EventID:         "evt-replay-1",
		EventType:       "diploma.updated.v1",
	})
	if err != nil {
		t.Fatalf("marshal dlq payload: %v", err)
	}

	produceRawKafkaRecord(t, env.kafkaBroker, dlqTopic, []byte("evt-replay-1"), dlqPayload)

	record := waitForKafkaRecord(t, sourceConsumer, 20*time.Second)
	if record.Topic != diplomaTopic {
		t.Fatalf("replayed topic = %s, want %s", record.Topic, diplomaTopic)
	}
	if string(record.Key) != "evt-replay-1" {
		t.Fatalf("replayed key = %s, want evt-replay-1", string(record.Key))
	}
	if string(record.Value) != string(sourceValue) {
		t.Fatalf("replayed value = %s, want %s", string(record.Value), string(sourceValue))
	}
	if !hasKafkaHeader(record.Headers, "x-dlq-replayed", "true") {
		t.Fatal("replayed record is missing x-dlq-replayed header")
	}

	cancel()
	assertConsumerStopped(t, errCh)
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	ensureDockerAvailable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	postgresContainer := startContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        postgresImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "diasoft_gateway",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
		},
		WaitingFor: wait.ForListeningPort(nat.Port("5432/tcp")),
	})

	redisContainer := startContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        redisImage,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort(nat.Port("6379/tcp")),
	})

	postgresDSN := postgresDSN(t, ctx, postgresContainer)
	dbPool, err := pgxpool.New(ctx, postgresDSN)
	if err != nil {
		t.Fatalf("create pgx pool: %v", err)
	}
	t.Cleanup(dbPool.Close)
	waitFor(t, 15*time.Second, func() error { return dbPool.Ping(ctx) })
	applyMigration(t, ctx, dbPool)

	redisAddress := redisAddress(t, ctx, redisContainer)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisAddress})
	t.Cleanup(func() { _ = redisClient.Close() })
	waitFor(t, 15*time.Second, func() error { return redisClient.Ping(ctx).Err() })

	return &integrationEnv{dbPool: dbPool, redisClient: redisClient}
}

func newKafkaIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	env := newIntegrationEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	redpandaContainer, err := redpanda.Run(ctx, redpandaImage)
	if err != nil {
		t.Fatalf("start redpanda container: %v", err)
	}
	t.Cleanup(func() {
		_ = redpandaContainer.Terminate(context.Background())
	})

	broker, err := redpandaContainer.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("resolve redpanda kafka seed broker: %v", err)
	}

	createKafkaTopics(t, broker, diplomaTopic, shareLinkTopic, dlqTopic)
	env.kafkaBroker = broker
	return env
}

func startContainer(t *testing.T, ctx context.Context, req testcontainers.ContainerRequest) testcontainers.Container {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container %s: %v", req.Image, err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})
	return container
}

func postgresDSN(t *testing.T, ctx context.Context, container testcontainers.Container) string {
	t.Helper()

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("postgres host: %v", err)
	}
	port, err := container.MappedPort(ctx, nat.Port("5432/tcp"))
	if err != nil {
		t.Fatalf("postgres mapped port: %v", err)
	}
	return fmt.Sprintf("postgres://postgres:postgres@%s:%s/diasoft_gateway?sslmode=disable", host, port.Port())
}

func redisAddress(t *testing.T, ctx context.Context, container testcontainers.Container) string {
	t.Helper()

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("redis host: %v", err)
	}
	port, err := container.MappedPort(ctx, nat.Port("6379/tcp"))
	if err != nil {
		t.Fatalf("redis mapped port: %v", err)
	}
	return fmt.Sprintf("%s:%s", host, port.Port())
}

func applyMigration(t *testing.T, ctx context.Context, dbPool *pgxpool.Pool) {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path")
	}
	migrationPath := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "0001_init.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}

	for _, statement := range strings.Split(string(content), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := dbPool.Exec(ctx, statement); err != nil {
			t.Fatalf("apply migration statement %q: %v", statement, err)
		}
	}
}

func mustExec(t *testing.T, dbPool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := dbPool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec query %q: %v", query, err)
	}
}

func ensureDockerAvailable(t *testing.T) {
	t.Helper()

	if os.Getenv("DOCKER_HOST") == "" {
		for _, candidate := range []string{"/var/run/docker.sock", filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock")} {
			if candidate == "" || strings.HasSuffix(candidate, "/docker.sock") == false {
				continue
			}
			if _, err := os.Stat(candidate); err == nil {
				t.Setenv("DOCKER_HOST", "unix://"+candidate)
				break
			}
		}
	}

	if os.Getenv("DOCKER_HOST") == "" {
		t.Skip("docker host is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		t.Skipf("docker is unavailable: %v", err)
	}
	if client != nil {
		_ = client.Close()
	}
}

func waitFor(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("wait condition failed: %v", lastErr)
	}
	t.Fatalf("wait condition failed: timeout exceeded")
}

func createKafkaTopics(t *testing.T, broker string, topics ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kgo.NewClient(kgo.SeedBrokers(broker))
	if err != nil {
		t.Fatalf("create kafka admin client: %v", err)
	}
	defer client.Close()

	admin := kadm.NewClient(client)
	responses, err := admin.CreateTopics(ctx, 1, 1, nil, topics...)
	if err != nil {
		t.Fatalf("create kafka topics: %v", err)
	}

	for _, topic := range topics {
		response, err := responses.On(topic, nil)
		if err != nil {
			t.Fatalf("read create topic response for %s: %v", topic, err)
		}
		if response.Err != nil && !errors.Is(response.Err, kerr.TopicAlreadyExists) {
			t.Fatalf("create topic %s: %v", topic, response.Err)
		}
	}
}

func startKafkaConsumer(t *testing.T, ctx context.Context, broker, group string, projector kafkainfra.EventProjector) <-chan error {
	t.Helper()

	consumer, err := kafkainfra.NewConsumer(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), config.KafkaConfig{
		Brokers:        []string{broker},
		ConsumerGroup:  group,
		DiplomaTopic:   diplomaTopic,
		ShareLinkTopic: shareLinkTopic,
		DLQTopic:       dlqTopic,
	}, projector, nil)
	if err != nil {
		t.Fatalf("create kafka consumer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Run(ctx)
	}()
	return errCh
}

func produceEvent(t *testing.T, broker, topic string, event usecase.EventEnvelope) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event envelope: %v", err)
	}

	client, err := kgo.NewClient(kgo.SeedBrokers(broker))
	if err != nil {
		t.Fatalf("create kafka producer client: %v", err)
	}
	defer client.Close()

	if err := client.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   []byte(event.EventID),
		Value: payload,
	}).FirstErr(); err != nil {
		t.Fatalf("produce kafka event to %s: %v", topic, err)
	}
}

func produceRawKafkaRecord(t *testing.T, broker, topic string, key, value []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kgo.NewClient(kgo.SeedBrokers(broker))
	if err != nil {
		t.Fatalf("create kafka raw producer client: %v", err)
	}
	defer client.Close()

	if err := client.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   append([]byte(nil), key...),
		Value: append([]byte(nil), value...),
	}).FirstErr(); err != nil {
		t.Fatalf("produce raw kafka record to %s: %v", topic, err)
	}
}

func newKafkaTopicConsumer(t *testing.T, broker, topic string, offset kgo.Offset) *kgo.Client {
	t.Helper()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(offset),
	)
	if err != nil {
		t.Fatalf("create kafka topic consumer for %s: %v", topic, err)
	}
	return client
}

func waitForKafkaRecord(t *testing.T, client *kgo.Client, timeout time.Duration) *kgo.Record {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		fetches := client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatalf("poll kafka topic: %v", errs)
		}
		iter := fetches.RecordIter()
		if !iter.Done() {
			return iter.Next()
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for kafka record: %v", err)
		}
	}
}

func startDLQReplayer(t *testing.T, ctx context.Context, broker, group string) <-chan error {
	t.Helper()

	replayer, err := kafkainfra.NewReplayer(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), config.KafkaConfig{
		Brokers:             []string{broker},
		ReplayConsumerGroup: group,
		DLQTopic:            dlqTopic,
	}, nil)
	if err != nil {
		t.Fatalf("create dlq replayer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- replayer.Run(ctx)
	}()
	return errCh
}

func assertConsumerStopped(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("consumer returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not stop within timeout")
	}
}

func uniqueConsumerGroup(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-")
	return fmt.Sprintf("itest-%s-%d", strings.ToLower(replacer.Replace(name)), time.Now().UnixNano())
}

func hasKafkaHeader(headers []kgo.RecordHeader, key, value string) bool {
	for _, header := range headers {
		if header.Key == key && string(header.Value) == value {
			return true
		}
	}
	return false
}
