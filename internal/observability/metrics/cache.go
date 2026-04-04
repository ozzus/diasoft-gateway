package metrics

import (
	"context"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

var _ port.VerificationCache = (*instrumentedVerificationCache)(nil)

type instrumentedVerificationCache struct {
	next    port.VerificationCache
	metrics *Service
}

func NewVerificationCache(next port.VerificationCache, metrics *Service) port.VerificationCache {
	if next == nil || metrics == nil {
		return next
	}
	return &instrumentedVerificationCache{next: next, metrics: metrics}
}

func (c *instrumentedVerificationCache) Get(ctx context.Context, key string) (domainverification.Result, bool, error) {
	result, found, err := c.next.Get(ctx, key)
	if err != nil {
		c.metrics.ObserveVerificationCache(key, "error")
		return domainverification.Result{}, false, err
	}
	if found {
		c.metrics.ObserveVerificationCache(key, "hit")
	} else {
		c.metrics.ObserveVerificationCache(key, "miss")
	}
	return result, found, nil
}

func (c *instrumentedVerificationCache) Set(ctx context.Context, key string, result domainverification.Result) error {
	return c.next.Set(ctx, key, result)
}

func (c *instrumentedVerificationCache) Delete(ctx context.Context, key string) error {
	return c.next.Delete(ctx, key)
}
