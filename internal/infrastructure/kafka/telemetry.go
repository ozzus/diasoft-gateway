package kafka

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

var kafkaTracer = otel.Tracer("github.com/ssovich/diasoft-gateway/internal/infrastructure/kafka")

type kafkaHeaderCarrier struct {
	headers *[]kgo.RecordHeader
}

func extractTraceContext(ctx context.Context, headers []kgo.RecordHeader) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	copyHeaders := append([]kgo.RecordHeader(nil), headers...)
	return otel.GetTextMapPropagator().Extract(ctx, kafkaHeaderCarrier{headers: &copyHeaders})
}

func injectTraceHeaders(ctx context.Context, headers []kgo.RecordHeader) []kgo.RecordHeader {
	carrierHeaders := append([]kgo.RecordHeader(nil), headers...)
	otel.GetTextMapPropagator().Inject(ctx, kafkaHeaderCarrier{headers: &carrierHeaders})
	return carrierHeaders
}

func (c kafkaHeaderCarrier) Get(key string) string {
	if c.headers == nil {
		return ""
	}
	for _, header := range *c.headers {
		if strings.EqualFold(header.Key, key) {
			return string(header.Value)
		}
	}
	return ""
}

func (c kafkaHeaderCarrier) Set(key, value string) {
	if c.headers == nil {
		return
	}
	for idx, header := range *c.headers {
		if strings.EqualFold(header.Key, key) {
			(*c.headers)[idx].Key = key
			(*c.headers)[idx].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c kafkaHeaderCarrier) Keys() []string {
	if c.headers == nil {
		return nil
	}
	keys := make([]string, 0, len(*c.headers))
	for _, header := range *c.headers {
		keys = append(keys, header.Key)
	}
	return keys
}

var _ propagation.TextMapCarrier = kafkaHeaderCarrier{}
