package usecase

import (
	"context"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

type RecordVerificationAudit struct {
	repository port.VerificationAuditRepository
}

type RecordVerificationAuditCommand struct {
	RequestType    string
	Token          string
	DiplomaNumber  string
	UniversityCode string
	RemoteIP       string
	Verdict        domainverification.Verdict
}

func NewRecordVerificationAudit(repository port.VerificationAuditRepository) *RecordVerificationAudit {
	return &RecordVerificationAudit{repository: repository}
}

func (u *RecordVerificationAudit) Run(ctx context.Context, cmd RecordVerificationAuditCommand) error {
	if u == nil || u.repository == nil {
		return nil
	}

	return u.repository.Save(ctx, port.VerificationAuditRecord{
		RequestType:    strings.TrimSpace(cmd.RequestType),
		Token:          strings.TrimSpace(cmd.Token),
		DiplomaNumber:  strings.TrimSpace(cmd.DiplomaNumber),
		UniversityCode: strings.ToUpper(strings.TrimSpace(cmd.UniversityCode)),
		RemoteIP:       strings.TrimSpace(cmd.RemoteIP),
		Verdict:        string(cmd.Verdict),
	})
}
