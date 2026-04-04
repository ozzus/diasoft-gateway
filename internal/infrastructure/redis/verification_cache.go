package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

var _ port.VerificationCache = (*VerificationCache)(nil)

type VerificationCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewVerificationCache(client *goredis.Client, ttl time.Duration) *VerificationCache {
	return &VerificationCache{client: client, ttl: ttl}
}

func (c *VerificationCache) Get(ctx context.Context, key string) (domainverification.Result, bool, error) {
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return domainverification.Result{}, false, nil
		}
		return domainverification.Result{}, false, fmt.Errorf("redis get %s: %w", key, err)
	}

	var result domainverification.Result
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return domainverification.Result{}, false, fmt.Errorf("decode cached verification result: %w", err)
	}

	return result, true, nil
}

func (c *VerificationCache) Set(ctx context.Context, key string, result domainverification.Result) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode cached verification result: %w", err)
	}
	if err := c.client.Set(ctx, key, payload, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	return nil
}

func (c *VerificationCache) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete %s: %w", key, err)
	}
	return nil
}
