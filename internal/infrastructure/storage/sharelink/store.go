package sharelink

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainsharelink "github.com/ssovich/diasoft-gateway/internal/domain/sharelink"
)

var _ port.ShareLinkRepository = (*Store)(nil)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Resolve(ctx context.Context, token string, now time.Time) (port.ShareLinkResolution, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return port.ShareLinkResolution{}, false, fmt.Errorf("begin share link resolve transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var resolution port.ShareLinkResolution
	var maxViews *int
	var usedViews int
	var status string

	err = tx.QueryRow(
		ctx,
		`select share_token, diploma_id::text, expires_at, max_views, used_views, status
		 from share_link_records
		 where share_token = $1
		 for update`,
		token,
	).Scan(&resolution.Token, &resolution.DiplomaID, &resolution.ExpiresAt, &maxViews, &usedViews, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return port.ShareLinkResolution{}, false, nil
		}
		return port.ShareLinkResolution{}, false, fmt.Errorf("query share link: %w", err)
	}

	resolution.State = domainsharelink.ParseState(status)
	if resolution.State != domainsharelink.StateActive {
		if err := tx.Commit(ctx); err != nil {
			return port.ShareLinkResolution{}, false, fmt.Errorf("commit resolved inactive share link: %w", err)
		}
		return resolution, true, nil
	}

	now = now.UTC()
	if !resolution.ExpiresAt.After(now) || viewLimitReached(maxViews, usedViews) {
		if _, err := tx.Exec(ctx, `update share_link_records set status = $2, updated_at = now() where share_token = $1`, token, string(domainsharelink.StateExpired)); err != nil {
			return port.ShareLinkResolution{}, false, fmt.Errorf("expire share link: %w", err)
		}
		resolution.State = domainsharelink.StateExpired
		if err := tx.Commit(ctx); err != nil {
			return port.ShareLinkResolution{}, false, fmt.Errorf("commit expired share link: %w", err)
		}
		return resolution, true, nil
	}

	nextUsedViews := usedViews + 1
	nextStatus := string(domainsharelink.StateActive)
	if viewLimitReached(maxViews, nextUsedViews) {
		nextStatus = string(domainsharelink.StateExpired)
	}

	if _, err := tx.Exec(
		ctx,
		`update share_link_records set used_views = $2, status = $3, updated_at = now() where share_token = $1`,
		token,
		nextUsedViews,
		nextStatus,
	); err != nil {
		return port.ShareLinkResolution{}, false, fmt.Errorf("increment share link usage: %w", err)
	}

	resolution.State = domainsharelink.StateActive
	if err := tx.Commit(ctx); err != nil {
		return port.ShareLinkResolution{}, false, fmt.Errorf("commit active share link: %w", err)
	}
	return resolution, true, nil
}

func viewLimitReached(maxViews *int, usedViews int) bool {
	return maxViews != nil && usedViews >= *maxViews
}
