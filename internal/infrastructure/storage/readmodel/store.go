package readmodel

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
)

var _ port.ReadModelRepository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

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
