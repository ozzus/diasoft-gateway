package sharelink

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrEmptyToken     = errors.New("empty share link token")
	ErrEmptyDiplomaID = errors.New("empty share link diploma id")
	ErrExpiredAt      = errors.New("share link expiration must be in the future")
)

type Link struct {
	token     string
	diplomaID string
	expiresAt time.Time
}

func NewLink(token, diplomaID string, expiresAt time.Time) (Link, error) {
	link := Link{
		token:     strings.TrimSpace(token),
		diplomaID: strings.TrimSpace(diplomaID),
		expiresAt: expiresAt.UTC(),
	}

	switch {
	case link.token == "":
		return Link{}, ErrEmptyToken
	case link.diplomaID == "":
		return Link{}, ErrEmptyDiplomaID
	case !link.expiresAt.After(time.Now().UTC()):
		return Link{}, ErrExpiredAt
	default:
		return link, nil
	}
}

func (l Link) Token() string     { return l.token }
func (l Link) DiplomaID() string { return l.diplomaID }
func (l Link) ExpiresAt() time.Time {
	return l.expiresAt
}
