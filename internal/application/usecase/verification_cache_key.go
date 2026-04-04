package usecase

import "strings"

func VerificationLookupCacheKey(diplomaNumber, universityCode string) string {
	return "verify:number:" + strings.ToUpper(strings.TrimSpace(universityCode)) + ":" + strings.TrimSpace(diplomaNumber)
}

func VerificationTokenCacheKey(verificationToken string) string {
	return "verify:token:" + strings.TrimSpace(verificationToken)
}
