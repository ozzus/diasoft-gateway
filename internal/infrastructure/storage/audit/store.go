package audit

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
)

var _ port.VerificationAuditRepository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Save(ctx context.Context, record port.VerificationAuditRecord) error {
	id, err := newUUID()
	if err != nil {
		return fmt.Errorf("generate audit id: %w", err)
	}

	_, err = s.pool.Exec(
		ctx,
		`insert into verification_audit (id, request_type, token, diploma_number, university_code, remote_ip, verdict, created_at)
		 values ($1::uuid, $2, $3, $4, $5, $6, $7, now())`,
		id,
		record.RequestType,
		nullable(record.Token),
		nullable(record.DiplomaNumber),
		nullable(record.UniversityCode),
		nullable(record.RemoteIP),
		record.Verdict,
	)
	if err != nil {
		return fmt.Errorf("save verification audit: %w", err)
	}

	return nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func newUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}
