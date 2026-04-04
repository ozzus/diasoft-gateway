package authuser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/privateapi"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) FindByLoginAndRole(ctx context.Context, login string, role privateapi.Role) (privateapi.User, bool, error) {
	return s.scanOne(
		ctx,
		`select id::text, login, password_hash, name, role, coalesce(organization_code, ''), coalesce(university_id::text, ''), coalesce(diploma_id::text, '')
		 from auth_users
		 where login = $1 and role = $2`,
		strings.TrimSpace(login),
		string(role),
	)
}

func (s *Store) FindByID(ctx context.Context, id string) (privateapi.User, bool, error) {
	return s.scanOne(
		ctx,
		`select id::text, login, password_hash, name, role, coalesce(organization_code, ''), coalesce(university_id::text, ''), coalesce(diploma_id::text, '')
		 from auth_users
		 where id::text = $1`,
		strings.TrimSpace(id),
	)
}

func (s *Store) scanOne(ctx context.Context, query string, args ...any) (privateapi.User, bool, error) {
	var user privateapi.User
	var role string

	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&user.ID,
		&user.Login,
		&user.PasswordHash,
		&user.Name,
		&role,
		&user.OrganizationCode,
		&user.UniversityID,
		&user.DiplomaID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return privateapi.User{}, false, nil
		}
		return privateapi.User{}, false, fmt.Errorf("query auth user: %w", err)
	}

	user.Role = privateapi.Role(role)
	return user, true, nil
}
