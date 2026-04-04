package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type readinessCheck func(context.Context) error

func registerPlatformRoutes(mux *http.ServeMux, ready readinessCheck) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
}

func readiness(checks ...readinessCheck) readinessCheck {
	return func(ctx context.Context) error {
		for _, check := range checks {
			if check == nil {
				continue
			}
			if err := check(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status_code", recorder.statusCode,
			"bytes_written", recorder.bytesWritten,
			"duration", time.Since(startedAt),
		}
		if spanCtx := trace.SpanContextFromContext(r.Context()); spanCtx.IsValid() {
			attrs = append(attrs,
				"trace_id", spanCtx.TraceID().String(),
				"span_id", spanCtx.SpanID().String(),
			)
		}
		logger.Info(
			"http request",
			attrs...,
		)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(payload []byte) (int, error) {
	written, err := r.ResponseWriter.Write(payload)
	r.bytesWritten += written
	return written, err
}
