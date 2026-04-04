package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainsharelink "github.com/ssovich/diasoft-gateway/internal/domain/sharelink"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

type resolveShareLinkRepoStub struct {
	resolution port.ShareLinkResolution
	found      bool
	err        error
}

func (s resolveShareLinkRepoStub) Save(context.Context, domainsharelink.Link) error {
	return nil
}

func (s resolveShareLinkRepoStub) Resolve(context.Context, string, time.Time) (port.ShareLinkResolution, bool, error) {
	return s.resolution, s.found, s.err
}

type diplomaRepoStub struct {
	byID  domainverification.Diploma
	found bool
	err   error
}

func (s diplomaRepoStub) FindByNumberAndUniversityCode(context.Context, string, string) (domainverification.Diploma, bool, error) {
	return domainverification.Diploma{}, false, nil
}

func (s diplomaRepoStub) FindByVerificationToken(context.Context, string) (domainverification.Diploma, bool, error) {
	return domainverification.Diploma{}, false, nil
}

func (s diplomaRepoStub) FindByID(context.Context, string) (domainverification.Diploma, bool, error) {
	return s.byID, s.found, s.err
}

func TestResolveShareLinkRunReturnsValidDiploma(t *testing.T) {
	t.Parallel()

	diploma := domainverification.MustNewDiploma("11111111-1111-1111-1111-111111111111", "MSU", "D-100", "И*** И***", "Computer Science", domainverification.StatusValid)
	useCase := NewResolveShareLink(
		resolveShareLinkRepoStub{
			resolution: port.ShareLinkResolution{DiplomaID: diploma.ID(), State: domainsharelink.StateActive},
			found:      true,
		},
		diplomaRepoStub{byID: diploma, found: true},
	)

	result, err := useCase.Run(context.Background(), ResolveShareLinkCommand{Token: "share-token"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Verdict != domainverification.VerdictValid {
		t.Fatalf("Run() verdict = %s, want %s", result.Verdict, domainverification.VerdictValid)
	}
}

func TestResolveShareLinkRunReturnsExpired(t *testing.T) {
	t.Parallel()

	useCase := NewResolveShareLink(
		resolveShareLinkRepoStub{
			resolution: port.ShareLinkResolution{State: domainsharelink.StateExpired},
			found:      true,
		},
		diplomaRepoStub{},
	)

	result, err := useCase.Run(context.Background(), ResolveShareLinkCommand{Token: "expired-token"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Verdict != domainverification.VerdictExpired {
		t.Fatalf("Run() verdict = %s, want %s", result.Verdict, domainverification.VerdictExpired)
	}
}

func TestResolveShareLinkRunReturnsNotFound(t *testing.T) {
	t.Parallel()

	useCase := NewResolveShareLink(resolveShareLinkRepoStub{found: false}, diplomaRepoStub{})

	result, err := useCase.Run(context.Background(), ResolveShareLinkCommand{Token: "missing-token"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Verdict != domainverification.VerdictNotFound {
		t.Fatalf("Run() verdict = %s, want %s", result.Verdict, domainverification.VerdictNotFound)
	}
}
