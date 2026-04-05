package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
)

type EventEnvelope struct {
	EventID      string          `json:"event_id"`
	EventType    string          `json:"event_type"`
	EventVersion string          `json:"event_version"`
	OccurredAt   time.Time       `json:"occurred_at"`
	Payload      json.RawMessage `json:"payload"`
}

type DiplomaLifecyclePayload struct {
	DiplomaID         string `json:"diploma_id"`
	VerificationToken string `json:"verification_token"`
	UniversityCode    string `json:"university_code"`
	DiplomaNumber     string `json:"diploma_number"`
	StudentName       string `json:"student_name"`
	StudentNameMasked string `json:"student_name_masked"`
	ProgramName       string `json:"program_name"`
	Status            string `json:"status"`
}

type ShareLinkLifecyclePayload struct {
	ShareToken string     `json:"share_token"`
	DiplomaID  string     `json:"diploma_id"`
	ExpiresAt  time.Time  `json:"expires_at"`
	MaxViews   *int       `json:"max_views"`
	UsedViews  int        `json:"used_views"`
	Status     string     `json:"status"`
}

type ProjectEvents struct {
	repository port.ReadModelRepository
	cache      port.VerificationCache
}

func NewProjectEvents(repository port.ReadModelRepository, cache port.VerificationCache) *ProjectEvents {
	return &ProjectEvents{repository: repository, cache: cache}
}

func (u *ProjectEvents) Handle(ctx context.Context, event EventEnvelope) error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("empty event id")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("empty event type")
	}

	processed, err := u.repository.IsProcessed(ctx, event.EventID)
	if err != nil {
		return err
	}
	if processed {
		return nil
	}

	switch event.EventType {
	case "diploma.created.v1", "diploma.updated.v1", "diploma.revoked.v1":
		var payload DiplomaLifecyclePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode diploma lifecycle payload: %w", err)
		}
		if err := u.repository.UpsertVerificationRecord(ctx, port.VerificationRecordProjection{
			DiplomaID:         payload.DiplomaID,
			VerificationToken: payload.VerificationToken,
			UniversityCode:    payload.UniversityCode,
			DiplomaNumber:     payload.DiplomaNumber,
			StudentName:       payload.StudentName,
			StudentNameMasked: payload.StudentNameMasked,
			ProgramName:       payload.ProgramName,
			Status:            payload.Status,
		}); err != nil {
			return err
		}
		if err := u.repository.UpsertStudentAuthUser(ctx, payload.DiplomaID, payload.DiplomaNumber, payload.StudentName); err != nil {
			return err
		}
		u.invalidateVerificationCache(ctx, payload.VerificationToken, payload.DiplomaNumber, payload.UniversityCode)
	case "sharelink.created.v1", "sharelink.revoked.v1", "sharelink.expired.v1":
		var payload ShareLinkLifecyclePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode share link lifecycle payload: %w", err)
		}
		if err := u.repository.UpsertShareLinkRecord(ctx, port.ShareLinkRecordProjection{
			ShareToken: payload.ShareToken,
			DiplomaID:  payload.DiplomaID,
			ExpiresAt:  payload.ExpiresAt,
			MaxViews:   payload.MaxViews,
			UsedViews:  payload.UsedViews,
			Status:     payload.Status,
		}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported event type: %s", event.EventType)
	}

	if err := u.repository.MarkProcessed(ctx, event.EventID, event.EventType); err != nil {
		return err
	}

	return nil
}

func (u *ProjectEvents) invalidateVerificationCache(ctx context.Context, verificationToken, diplomaNumber, universityCode string) {
	if u.cache == nil {
		return
	}
	_ = u.cache.Delete(ctx, VerificationTokenCacheKey(verificationToken))
	_ = u.cache.Delete(ctx, VerificationLookupCacheKey(diplomaNumber, universityCode))
}
