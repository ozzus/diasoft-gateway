package diploma

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

var _ port.DiplomaRepository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) FindByNumberAndUniversityCode(ctx context.Context, diplomaNumber, universityCode string) (domainverification.Diploma, bool, error) {
	return s.scanOne(
		ctx,
		`select diploma_id::text, university_code, diploma_number, student_name_masked, program_name, status
		 from verification_records
		 where diploma_number = $1 and university_code = $2`,
		strings.TrimSpace(diplomaNumber),
		strings.ToUpper(strings.TrimSpace(universityCode)),
	)
}

func (s *Store) FindByVerificationToken(ctx context.Context, verificationToken string) (domainverification.Diploma, bool, error) {
	return s.scanOne(
		ctx,
		`select diploma_id::text, university_code, diploma_number, student_name_masked, program_name, status
		 from verification_records
		 where verification_token = $1`,
		strings.TrimSpace(verificationToken),
	)
}

func (s *Store) FindByID(ctx context.Context, diplomaID string) (domainverification.Diploma, bool, error) {
	return s.scanOne(
		ctx,
		`select diploma_id::text, university_code, diploma_number, student_name_masked, program_name, status
		 from verification_records
		 where diploma_id::text = $1`,
		strings.TrimSpace(diplomaID),
	)
}

func (s *Store) scanOne(ctx context.Context, query string, args ...any) (domainverification.Diploma, bool, error) {
	var (
		id             string
		universityCode string
		diplomaNumber  string
		ownerName      string
		programName    string
		status         string
	)

	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&id,
		&universityCode,
		&diplomaNumber,
		&ownerName,
		&programName,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domainverification.Diploma{}, false, nil
		}
		return domainverification.Diploma{}, false, fmt.Errorf("query verification record: %w", err)
	}

	diploma, err := domainverification.NewDiploma(id, universityCode, diplomaNumber, ownerName, programName, domainverification.Status(status))
	if err != nil {
		return domainverification.Diploma{}, false, fmt.Errorf("build diploma from record: %w", err)
	}

	return diploma, true, nil
}
