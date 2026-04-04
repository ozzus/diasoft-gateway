package usecase

import (
	"context"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/application/port"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
)

type Verify struct {
	diplomaRepository port.DiplomaRepository
	cache             port.VerificationCache
}

type VerifyCommand struct {
	DiplomaNumber  string
	UniversityCode string
}

type VerifyByTokenCommand struct {
	VerificationToken string
}

func NewVerify(diplomaRepository port.DiplomaRepository, cache port.VerificationCache) *Verify {
	return &Verify{diplomaRepository: diplomaRepository, cache: cache}
}

func (u *Verify) Run(ctx context.Context, cmd VerifyCommand) (domainverification.Result, error) {
	diplomaNumber := strings.TrimSpace(cmd.DiplomaNumber)
	universityCode := strings.ToUpper(strings.TrimSpace(cmd.UniversityCode))
	cacheKey := VerificationLookupCacheKey(diplomaNumber, universityCode)

	if result, found, err := u.fromCache(ctx, cacheKey); err != nil {
		return domainverification.Result{}, err
	} else if found {
		return result, nil
	}

	diploma, found, err := u.diplomaRepository.FindByNumberAndUniversityCode(ctx, diplomaNumber, universityCode)
	if err != nil {
		return domainverification.Result{}, err
	}

	result := buildResult(diploma, found)
	u.toCache(ctx, cacheKey, result)
	return result, nil
}

func (u *Verify) RunByToken(ctx context.Context, cmd VerifyByTokenCommand) (domainverification.Result, error) {
	verificationToken := strings.TrimSpace(cmd.VerificationToken)
	cacheKey := VerificationTokenCacheKey(verificationToken)

	if result, found, err := u.fromCache(ctx, cacheKey); err != nil {
		return domainverification.Result{}, err
	} else if found {
		return result, nil
	}

	diploma, found, err := u.diplomaRepository.FindByVerificationToken(ctx, verificationToken)
	if err != nil {
		return domainverification.Result{}, err
	}

	result := buildResult(diploma, found)
	u.toCache(ctx, cacheKey, result)
	if found {
		u.toCache(ctx, VerificationLookupCacheKey(diploma.Number(), diploma.UniversityCode()), result)
	}
	return result, nil
}

func (u *Verify) fromCache(ctx context.Context, key string) (domainverification.Result, bool, error) {
	if u.cache == nil {
		return domainverification.Result{}, false, nil
	}
	return u.cache.Get(ctx, key)
}

func (u *Verify) toCache(ctx context.Context, key string, result domainverification.Result) {
	if u.cache == nil {
		return
	}
	_ = u.cache.Set(ctx, key, result)
}

func buildResult(diploma domainverification.Diploma, found bool) domainverification.Result {
	if !found {
		return domainverification.Result{Verdict: domainverification.VerdictNotFound}
	}

	result := domainverification.Result{
		DiplomaNumber:  diploma.Number(),
		UniversityCode: diploma.UniversityCode(),
		OwnerNameMask:  diploma.OwnerName(),
		Program:        diploma.Program(),
	}

	if diploma.Status() == domainverification.StatusRevoked {
		result.Verdict = domainverification.VerdictRevoked
		return result
	}

	result.Verdict = domainverification.VerdictValid
	return result
}
