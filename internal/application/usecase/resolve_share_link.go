package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainsharelink "github.com/ssovich/diasoft-gateway/internal/domain/sharelink"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

type ResolveShareLink struct {
	shareLinkRepository port.ShareLinkRepository
	diplomaRepository   port.DiplomaRepository
}

type ResolveShareLinkCommand struct {
	Token string
}

func NewResolveShareLink(shareLinkRepository port.ShareLinkRepository, diplomaRepository port.DiplomaRepository) *ResolveShareLink {
	return &ResolveShareLink{
		shareLinkRepository: shareLinkRepository,
		diplomaRepository:   diplomaRepository,
	}
}

func (u *ResolveShareLink) Run(ctx context.Context, cmd ResolveShareLinkCommand) (domainverification.Result, error) {
	token := strings.TrimSpace(cmd.Token)
	if token == "" {
		return domainverification.Result{Verdict: domainverification.VerdictNotFound}, nil
	}

	resolution, found, err := u.shareLinkRepository.Resolve(ctx, token, time.Now().UTC())
	if err != nil {
		return domainverification.Result{}, err
	}
	if !found {
		return domainverification.Result{Verdict: domainverification.VerdictNotFound}, nil
	}

	switch resolution.State {
	case domainsharelink.StateExpired:
		return domainverification.Result{Verdict: domainverification.VerdictExpired}, nil
	case domainsharelink.StateRevoked:
		return domainverification.Result{Verdict: domainverification.VerdictNotFound}, nil
	case domainsharelink.StateActive:
		diploma, diplomaFound, err := u.diplomaRepository.FindByID(ctx, resolution.DiplomaID)
		if err != nil {
			return domainverification.Result{}, err
		}
		return buildResult(diploma, diplomaFound), nil
	default:
		return domainverification.Result{Verdict: domainverification.VerdictNotFound}, nil
	}
}
