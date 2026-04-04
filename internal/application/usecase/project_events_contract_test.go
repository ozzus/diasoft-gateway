package usecase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type kafkaContractSchema struct {
	Title      string   `json:"title"`
	Required   []string `json:"required"`
	Properties struct {
		EventType struct {
			Const string `json:"const"`
		} `json:"event_type"`
		AggregateType struct {
			Const string `json:"const"`
		} `json:"aggregate_type"`
		Payload struct {
			Required []string `json:"required"`
		} `json:"payload"`
	} `json:"properties"`
}

func TestRegistryKafkaSchemasStayAlignedWithProjector(t *testing.T) {
	schemasDir := os.Getenv("REGISTRY_KAFKA_CONTRACTS_DIR")
	if schemasDir == "" {
		root := repositoryRoot(t)
		schemasDir = filepath.Join(root, "contracts", "upstream", "registry", "kafka")
	}

	schemaPaths, err := filepath.Glob(filepath.Join(schemasDir, "*.json"))
	if err != nil {
		t.Fatalf("glob schemas: %v", err)
	}
	if len(schemaPaths) == 0 {
		t.Fatalf("no upstream registry kafka schemas found in %s", schemasDir)
	}

	handledEventTypes := map[string]struct{}{
		"diploma.created.v1":   {},
		"diploma.updated.v1":   {},
		"diploma.revoked.v1":   {},
		"sharelink.created.v1": {},
		"sharelink.revoked.v1": {},
	}
	ignoredEventTypes := map[string]struct{}{
		"import.completed.v1": {},
	}

	for _, schemaPath := range schemaPaths {
		rawSchema, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("read schema %s: %v", schemaPath, err)
		}

		var schema kafkaContractSchema
		if err := json.Unmarshal(rawSchema, &schema); err != nil {
			t.Fatalf("decode schema %s: %v", schemaPath, err)
		}

		requireContainsAll(t, schema.Required, "event_id", "event_type", "event_version", "occurred_at", "payload")

		eventType := schema.Properties.EventType.Const
		switch {
		case hasKey(handledEventTypes, eventType):
			switch schema.Properties.AggregateType.Const {
			case "diploma":
				requireContainsAll(
					t,
					schema.Properties.Payload.Required,
					"diploma_id",
					"verification_token",
					"university_code",
					"diploma_number",
					"student_name_masked",
					"program_name",
					"status",
				)
			case "share_link":
				requireContainsAll(
					t,
					schema.Properties.Payload.Required,
					"share_token",
					"diploma_id",
					"status",
				)
			default:
				t.Fatalf("unexpected aggregate_type %q in %s", schema.Properties.AggregateType.Const, schemaPath)
			}
		case hasKey(ignoredEventTypes, eventType):
			// Gateway intentionally ignores non-read-model events such as import lifecycle notifications.
		default:
			t.Fatalf("unexpected upstream event type %q in %s", eventType, schemaPath)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func requireContainsAll(t *testing.T, values []string, expected ...string) {
	t.Helper()
	for _, item := range expected {
		if !hasString(values, item) {
			t.Fatalf("expected %q in %v", item, values)
		}
	}
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasKey(values map[string]struct{}, target string) bool {
	_, ok := values[target]
	return ok
}
