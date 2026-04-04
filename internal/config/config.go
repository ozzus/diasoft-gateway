package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type Config struct {
	Env       string          `yaml:"env" env:"ENV" env-default:"local"`
	Log       LogConfig       `yaml:"log"`
	HTTP      HTTPConfig      `yaml:"http"`
	Auth      AuthConfig      `yaml:"auth"`
	Registry  RegistryConfig  `yaml:"registry"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Tracing   TracingConfig   `yaml:"tracing"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Kafka     KafkaConfig     `yaml:"kafka"`
}

type LogConfig struct {
	Level string `yaml:"level" env:"LOG_LEVEL" env-default:"info"`
}

type HTTPConfig struct {
	Address        string   `yaml:"address" env:"HTTP_ADDRESS" env-default:":8080"`
	TrustedProxies []string `yaml:"trusted_proxies" env:"HTTP_TRUSTED_PROXIES" env-separator:","`
	AllowedOrigins []string `yaml:"allowed_origins" env:"HTTP_ALLOWED_ORIGINS" env-separator:","`
}

type AuthConfig struct {
	JWTSecret string        `yaml:"jwt_secret" env:"AUTH_JWT_SECRET" env-default:"diasoft-dev-secret"`
	TokenTTL  time.Duration `yaml:"token_ttl" env:"AUTH_TOKEN_TTL" env-default:"12h"`
}

type RegistryConfig struct {
	InternalBaseURL string        `yaml:"internal_base_url" env:"REGISTRY_INTERNAL_BASE_URL" env-default:"http://localhost:8081"`
	ServiceToken    string        `yaml:"service_token" env:"REGISTRY_SERVICE_TOKEN"`
	Timeout         time.Duration `yaml:"timeout" env:"REGISTRY_TIMEOUT" env-default:"15s"`
}

type MetricsConfig struct {
	Address string `yaml:"address" env:"METRICS_ADDRESS" env-default:":9090"`
}

type TracingConfig struct {
	Enabled          bool    `yaml:"enabled" env:"TRACING_ENABLED" env-default:"false"`
	OTLPEndpoint     string  `yaml:"otlp_endpoint" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	Insecure         bool    `yaml:"insecure" env:"OTEL_EXPORTER_OTLP_INSECURE" env-default:"true"`
	SampleRatio      float64 `yaml:"sample_ratio" env:"TRACING_SAMPLE_RATIO" env-default:"1"`
	ServiceNamespace string  `yaml:"service_namespace" env:"TRACING_SERVICE_NAMESPACE" env-default:"diasoft"`
}

type DatabaseConfig struct {
	URL      string `yaml:"url" env:"DATABASE_URL" env-default:"postgres://postgres:postgres@localhost:5432/diasoft_gateway?sslmode=disable"`
	MaxConns int32  `yaml:"max_conns" env:"DATABASE_MAX_CONNS" env-default:"10"`
}

type RedisConfig struct {
	Addr         string        `yaml:"addr" env:"REDIS_ADDR" env-default:"localhost:6379"`
	Password     string        `yaml:"password" env:"REDIS_PASSWORD"`
	DB           int           `yaml:"db" env:"REDIS_DB" env-default:"0"`
	VerifyTTL    time.Duration `yaml:"verify_ttl" env:"REDIS_VERIFY_TTL" env-default:"5m"`
	DialTimeout  time.Duration `yaml:"dial_timeout" env:"REDIS_DIAL_TIMEOUT" env-default:"5s"`
	ReadTimeout  time.Duration `yaml:"read_timeout" env:"REDIS_READ_TIMEOUT" env-default:"3s"`
	WriteTimeout time.Duration `yaml:"write_timeout" env:"REDIS_WRITE_TIMEOUT" env-default:"3s"`
	PoolSize     int           `yaml:"pool_size" env:"REDIS_POOL_SIZE" env-default:"10"`
	MinIdleConns int           `yaml:"min_idle_conns" env:"REDIS_MIN_IDLE_CONNS" env-default:"2"`
	MaxRetries   int           `yaml:"max_retries" env:"REDIS_MAX_RETRIES" env-default:"3"`
}

type RateLimitConfig struct {
	Enabled  bool          `yaml:"enabled" env:"RATE_LIMIT_ENABLED" env-default:"true"`
	Requests int64         `yaml:"requests" env:"RATE_LIMIT_REQUESTS" env-default:"60"`
	Window   time.Duration `yaml:"window" env:"RATE_LIMIT_WINDOW" env-default:"1m"`
	Prefix   string        `yaml:"prefix" env:"RATE_LIMIT_PREFIX" env-default:"rl"`
}

type KafkaConfig struct {
	Brokers                []string      `yaml:"brokers" env:"KAFKA_BROKERS" env-separator:","`
	Username               string        `yaml:"username" env:"KAFKA_USERNAME"`
	Password               string        `yaml:"password" env:"KAFKA_PASSWORD"`
	SASLEnabled            bool          `yaml:"sasl_enabled" env:"KAFKA_SASL_ENABLED" env-default:"false"`
	SASLMechanism          string        `yaml:"sasl_mechanism" env:"KAFKA_SASL_MECHANISM" env-default:"SCRAM-SHA-512"`
	TLSEnabled             bool          `yaml:"tls_enabled" env:"KAFKA_TLS_ENABLED" env-default:"false"`
	ConsumerGroup          string        `yaml:"consumer_group" env:"KAFKA_CONSUMER_GROUP" env-default:"diasoft-gateway-consumer"`
	ProjectRetryAttempts   int           `yaml:"project_retry_attempts" env:"KAFKA_PROJECT_RETRY_ATTEMPTS" env-default:"3"`
	ProjectRetryBackoff    time.Duration `yaml:"project_retry_backoff" env:"KAFKA_PROJECT_RETRY_BACKOFF" env-default:"250ms"`
	ProjectRetryMaxBackoff time.Duration `yaml:"project_retry_max_backoff" env:"KAFKA_PROJECT_RETRY_MAX_BACKOFF" env-default:"2s"`
	ReplayConsumerGroup    string        `yaml:"replay_consumer_group" env:"KAFKA_REPLAY_CONSUMER_GROUP" env-default:"diasoft-gateway-dlq-replayer"`
	ReplaySourceTopics     []string      `yaml:"replay_source_topics" env:"KAFKA_REPLAY_SOURCE_TOPICS" env-separator:","`
	ReplayEventTypes       []string      `yaml:"replay_event_types" env:"KAFKA_REPLAY_EVENT_TYPES" env-separator:","`
	ReplayFailureStages    []string      `yaml:"replay_failure_stages" env:"KAFKA_REPLAY_FAILURE_STAGES" env-separator:","`
	ReplayEventIDs         []string      `yaml:"replay_event_ids" env:"KAFKA_REPLAY_EVENT_IDS" env-separator:","`
	ReplayMaxMessages      int64         `yaml:"replay_max_messages" env:"KAFKA_REPLAY_MAX_MESSAGES" env-default:"0"`
	ReplayDryRun           bool          `yaml:"replay_dry_run" env:"KAFKA_REPLAY_DRY_RUN" env-default:"false"`
	DiplomaTopic           string        `yaml:"diploma_topic" env:"KAFKA_DIPLOMA_TOPIC" env-default:"diploma.lifecycle.v1"`
	ShareLinkTopic         string        `yaml:"share_link_topic" env:"KAFKA_SHARELINK_TOPIC" env-default:"sharelink.lifecycle.v1"`
	DLQTopic               string        `yaml:"dlq_topic" env:"KAFKA_DLQ_TOPIC" env-default:"gateway.dlq.v1"`
}

var dotenvLoadOnce sync.Once

func Load() (Config, error) {
	loadDotEnv()

	path, explicit := fetchConfigPath()
	if path == "" {
		return loadEnvOnly()
	}

	cfg, err := LoadByPath(path)
	if err == nil {
		return cfg, nil
	}

	if explicit {
		return Config{}, err
	}

	if os.IsNotExist(err) || strings.Contains(err.Error(), "cannot read config file") {
		return loadEnvOnly()
	}

	return Config{}, err
}

func LoadByPath(configPath string) (Config, error) {
	loadDotEnv()

	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("cannot read config file: %w", err)
	}

	var cfg Config
	if err := cleanenv.ParseYAML(strings.NewReader(os.ExpandEnv(string(rawConfig))), &cfg); err != nil {
		return Config{}, fmt.Errorf("cannot parse config: %w", err)
	}
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("cannot read env overrides: %w", err)
	}

	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return &cfg
}

func fetchConfigPath() (string, bool) {
	var res string
	explicit := false
	if flag.Lookup("config") == nil {
		flag.StringVar(&res, "config", "", "path to config file")
	}
	if !flag.Parsed() {
		flag.Parse()
	}
	if lookup := flag.Lookup("config"); lookup != nil && lookup.Value.String() != "" {
		res = lookup.Value.String()
		explicit = true
	}
	if res == "" {
		if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
			res = envPath
			explicit = true
		}
	}
	if res == "" {
		res = "config/local.yaml"
	}
	return res, explicit
}

func loadDotEnv() {
	dotenvLoadOnce.Do(func() {
		_ = godotenv.Load()
	})
}

func loadEnvOnly() (Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("cannot read env config: %w", err)
	}

	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) normalize() {
	c.Env = strings.TrimSpace(c.Env)
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	c.HTTP.Address = strings.TrimSpace(c.HTTP.Address)
	c.HTTP.TrustedProxies = compactStrings(c.HTTP.TrustedProxies)
	c.HTTP.AllowedOrigins = compactStrings(c.HTTP.AllowedOrigins)
	c.Auth.JWTSecret = strings.TrimSpace(c.Auth.JWTSecret)
	c.Registry.InternalBaseURL = strings.TrimRight(strings.TrimSpace(c.Registry.InternalBaseURL), "/")
	c.Registry.ServiceToken = strings.TrimSpace(c.Registry.ServiceToken)
	c.Metrics.Address = strings.TrimSpace(c.Metrics.Address)
	c.Tracing.OTLPEndpoint = strings.TrimSpace(c.Tracing.OTLPEndpoint)
	c.Tracing.ServiceNamespace = strings.TrimSpace(c.Tracing.ServiceNamespace)
	c.Database.URL = strings.TrimSpace(c.Database.URL)
	c.Redis.Addr = strings.TrimSpace(c.Redis.Addr)
	c.RateLimit.Prefix = strings.TrimSpace(c.RateLimit.Prefix)
	for i, broker := range c.Kafka.Brokers {
		c.Kafka.Brokers[i] = strings.TrimSpace(broker)
	}
	c.Kafka.Username = strings.TrimSpace(c.Kafka.Username)
	c.Kafka.Password = strings.TrimSpace(c.Kafka.Password)
	c.Kafka.SASLMechanism = strings.ToUpper(strings.TrimSpace(c.Kafka.SASLMechanism))
	c.Kafka.ConsumerGroup = strings.TrimSpace(c.Kafka.ConsumerGroup)
	c.Kafka.ReplayConsumerGroup = strings.TrimSpace(c.Kafka.ReplayConsumerGroup)
	c.Kafka.ReplaySourceTopics = compactStrings(c.Kafka.ReplaySourceTopics)
	c.Kafka.ReplayEventTypes = compactStrings(c.Kafka.ReplayEventTypes)
	c.Kafka.ReplayFailureStages = compactStrings(c.Kafka.ReplayFailureStages)
	c.Kafka.ReplayEventIDs = compactStrings(c.Kafka.ReplayEventIDs)
	c.Kafka.DiplomaTopic = strings.TrimSpace(c.Kafka.DiplomaTopic)
	c.Kafka.ShareLinkTopic = strings.TrimSpace(c.Kafka.ShareLinkTopic)
	c.Kafka.DLQTopic = strings.TrimSpace(c.Kafka.DLQTopic)
}

func (c Config) validate() error {
	if c.HTTP.Address == "" {
		return fmt.Errorf("http.address is required")
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret is required")
	}
	if c.Auth.TokenTTL <= 0 {
		return fmt.Errorf("auth.token_ttl must be greater than zero")
	}
	if c.Registry.InternalBaseURL == "" {
		return fmt.Errorf("registry.internal_base_url is required")
	}
	if c.Registry.Timeout <= 0 {
		return fmt.Errorf("registry.timeout must be greater than zero")
	}
	if c.Database.URL == "" {
		return fmt.Errorf("database.url is required")
	}
	if c.Database.MaxConns <= 0 {
		return fmt.Errorf("database.max_conns must be greater than zero")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis.pool_size must be greater than zero")
	}
	if c.RateLimit.Enabled {
		if c.RateLimit.Requests <= 0 {
			return fmt.Errorf("rate_limit.requests must be greater than zero")
		}
		if c.RateLimit.Window <= 0 {
			return fmt.Errorf("rate_limit.window must be greater than zero")
		}
	}
	if c.Kafka.ReplayMaxMessages < 0 {
		return fmt.Errorf("kafka.replay_max_messages cannot be negative")
	}
	if c.Kafka.SASLEnabled {
		if c.Kafka.Username == "" || c.Kafka.Password == "" {
			return fmt.Errorf("kafka.username and kafka.password are required when kafka.sasl_enabled is true")
		}
		switch c.Kafka.SASLMechanism {
		case "SCRAM-SHA-256", "SCRAM-SHA-512":
		default:
			return fmt.Errorf("kafka.sasl_mechanism must be SCRAM-SHA-256 or SCRAM-SHA-512")
		}
	}
	if c.Kafka.ProjectRetryAttempts < 0 {
		return fmt.Errorf("kafka.project_retry_attempts cannot be negative")
	}
	if c.Kafka.ProjectRetryBackoff < 0 {
		return fmt.Errorf("kafka.project_retry_backoff cannot be negative")
	}
	if c.Kafka.ProjectRetryMaxBackoff < 0 {
		return fmt.Errorf("kafka.project_retry_max_backoff cannot be negative")
	}
	if c.Kafka.ProjectRetryMaxBackoff > 0 && c.Kafka.ProjectRetryBackoff > c.Kafka.ProjectRetryMaxBackoff {
		return fmt.Errorf("kafka.project_retry_backoff cannot be greater than kafka.project_retry_max_backoff")
	}
	if c.Tracing.SampleRatio < 0 || c.Tracing.SampleRatio > 1 {
		return fmt.Errorf("tracing.sample_ratio must be between 0 and 1")
	}
	if c.Tracing.Enabled && c.Tracing.OTLPEndpoint == "" {
		return fmt.Errorf("tracing.otlp_endpoint is required when tracing is enabled")
	}
	return nil
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
