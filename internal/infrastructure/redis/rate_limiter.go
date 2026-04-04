package redis

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
)

var _ port.RateLimiter = (*RateLimiter)(nil)

var rateLimitScript = goredis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])

local current = redis.call("INCR", key)
if current == 1 then
  redis.call("EXPIRE", key, window)
end

if current > limit then
  return 0
end
return 1
`)

type RateLimiter struct {
	client *goredis.Client
	prefix string
	limit  int64
	window time.Duration
}

func NewRateLimiter(client *goredis.Client, prefix string, limit int64, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client: client,
		prefix: strings.TrimSpace(prefix),
		limit:  limit,
		window: window,
	}
}

func (l *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if l == nil || l.client == nil || l.limit <= 0 || l.window <= 0 {
		return true, nil
	}

	windowSeconds := int64(math.Ceil(l.window.Seconds()))
	if windowSeconds <= 0 {
		windowSeconds = 1
	}

	result, err := rateLimitScript.Run(ctx, l.client, []string{l.buildKey(key)}, l.limit, windowSeconds).Int()
	if err != nil {
		return false, fmt.Errorf("run redis rate limit script: %w", err)
	}

	return result == 1, nil
}

func (l *RateLimiter) buildKey(key string) string {
	if l.prefix == "" {
		return strings.TrimSpace(key)
	}
	return fmt.Sprintf("%s:%s", l.prefix, strings.TrimSpace(key))
}
