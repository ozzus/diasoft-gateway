package port

import (
	"context"
	"time"

	domainsharelink "github.com/ssovich/diasoft-gateway/internal/domain/sharelink"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

type DiplomaRepository interface {
	FindByNumberAndUniversityCode(ctx context.Context, diplomaNumber, universityCode string) (domainverification.Diploma, bool, error)
	FindByVerificationToken(ctx context.Context, verificationToken string) (domainverification.Diploma, bool, error)
	FindByID(ctx context.Context, diplomaID string) (domainverification.Diploma, bool, error)
}

type ShareLinkResolution struct {
	Token     string
	DiplomaID string
	ExpiresAt time.Time
	State     domainsharelink.State
}

type ShareLinkRepository interface {
	Resolve(ctx context.Context, token string, now time.Time) (ShareLinkResolution, bool, error)
}

type VerificationCache interface {
	Get(ctx context.Context, key string) (domainverification.Result, bool, error)
	Set(ctx context.Context, key string, result domainverification.Result) error
	Delete(ctx context.Context, key string) error
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type VerificationAuditRecord struct {
	RequestType    string
	Token          string
	DiplomaNumber  string
	UniversityCode string
	RemoteIP       string
	Verdict        string
}

type VerificationAuditRepository interface {
	Save(ctx context.Context, record VerificationAuditRecord) error
}

type VerificationRecordProjection struct {
	DiplomaID         string
	VerificationToken string
	UniversityCode    string
	DiplomaNumber     string
	StudentName       string
	StudentNameMasked string
	ProgramName       string
	Status            string
}

type ShareLinkRecordProjection struct {
	ShareToken string
	DiplomaID  string
	ExpiresAt  time.Time
	MaxViews   *int
	UsedViews  int
	Status     string
}

type ReadModelRepository interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID, eventType string) error
	UpsertVerificationRecord(ctx context.Context, record VerificationRecordProjection) error
	UpsertShareLinkRecord(ctx context.Context, record ShareLinkRecordProjection) error
	UpsertStudentAuthUser(ctx context.Context, diplomaID, diplomaNumber, studentName string) error
}
