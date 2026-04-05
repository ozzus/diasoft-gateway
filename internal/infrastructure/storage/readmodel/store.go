package readmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
)

var _ port.ReadModelRepository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

const defaultStudentPasswordHash = "$2a$10$7f2jyY18zCn5LeyJt2jP9Of0WixOkOTahKsBgd98iWlu9M6QXtMAy"

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, `select true from processed_events where event_id = $1`, eventID).Scan(&found)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("query processed event: %w", err)
	}
	return found, nil
}

func (s *Store) MarkProcessed(ctx context.Context, eventID, eventType string) error {
	_, err := s.pool.Exec(ctx, `insert into processed_events (event_id, event_type, processed_at) values ($1, $2, now()) on conflict (event_id) do nothing`, eventID, eventType)
	if err != nil {
		return fmt.Errorf("mark processed event: %w", err)
	}
	return nil
}

func (s *Store) UpsertVerificationRecord(ctx context.Context, record port.VerificationRecordProjection) error {
	_, err := s.pool.Exec(
		ctx,
		`insert into verification_records (diploma_id, verification_token, university_code, diploma_number, student_name_masked, program_name, status, updated_at)
		 values ($1::uuid, $2, $3, $4, $5, $6, $7, now())
		 on conflict (diploma_id) do update set
		   verification_token = excluded.verification_token,
		   university_code = excluded.university_code,
		   diploma_number = excluded.diploma_number,
		   student_name_masked = excluded.student_name_masked,
		   program_name = excluded.program_name,
		   status = excluded.status,
		   updated_at = now()`,
		record.DiplomaID,
		record.VerificationToken,
		record.UniversityCode,
		record.DiplomaNumber,
		record.StudentNameMasked,
		record.ProgramName,
		record.Status,
	)
	if err != nil {
		return fmt.Errorf("upsert verification record: %w", err)
	}
	return nil
}

func (s *Store) UpsertStudentAuthUser(ctx context.Context, diplomaID, diplomaNumber, studentName string) error {
	if strings.TrimSpace(diplomaID) == "" || strings.TrimSpace(diplomaNumber) == "" || strings.TrimSpace(studentName) == "" {
		return nil
	}

	_, err := s.pool.Exec(
		ctx,
		`insert into auth_users (id, login, password_hash, name, role, diploma_id, created_at, updated_at)
		 values ($1::uuid, $2, $3, $4, 'student', $5::uuid, now(), now())
		 on conflict (login, role) do update set
		   name = excluded.name,
		   diploma_id = excluded.diploma_id,
		   updated_at = now()`,
		diplomaID,
		strings.TrimSpace(diplomaNumber),
		defaultStudentPasswordHash,
		strings.TrimSpace(studentName),
		diplomaID,
	)
	if err != nil {
		return fmt.Errorf("upsert student auth user: %w", err)
	}
	return nil
}

func (s *Store) UpsertShareLinkRecord(ctx context.Context, record port.ShareLinkRecordProjection) error {
	_, err := s.pool.Exec(
		ctx,
		`insert into share_link_records (share_token, diploma_id, expires_at, max_views, used_views, status, updated_at)
		 values ($1, $2::uuid, $3, $4, $5, $6, now())
		 on conflict (share_token) do update set
		   diploma_id = excluded.diploma_id,
		   expires_at = excluded.expires_at,
		   max_views = excluded.max_views,
		   used_views = excluded.used_views,
		   status = excluded.status,
		   updated_at = now()`,
		record.ShareToken,
		record.DiplomaID,
		record.ExpiresAt,
		record.MaxViews,
		record.UsedViews,
		record.Status,
	)
	if err != nil {
		return fmt.Errorf("upsert share link record: %w", err)
	}
	return nil
}
