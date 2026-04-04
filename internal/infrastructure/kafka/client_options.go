package kafka

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

func baseClientOptions(cfg config.KafkaConfig) ([]kgo.Opt, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
	}

	if cfg.TLSEnabled {
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
		}))
	}

	if cfg.SASLEnabled {
		mechanism, err := kafkaSASLMechanism(cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.SASL(mechanism))
	}

	return opts, nil
}

func kafkaSASLMechanism(cfg config.KafkaConfig) (sasl.Mechanism, error) {
	auth := scram.Auth{
		User: cfg.Username,
		Pass: cfg.Password,
	}

	switch strings.ToUpper(strings.TrimSpace(cfg.SASLMechanism)) {
	case "SCRAM-SHA-256":
		return auth.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512", "":
		return auth.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("unsupported kafka sasl mechanism %q", cfg.SASLMechanism)
	}
}
