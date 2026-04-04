package verification

type Verdict string

const (
	VerdictValid    Verdict = "valid"
	VerdictRevoked  Verdict = "revoked"
	VerdictExpired  Verdict = "expired"
	VerdictNotFound Verdict = "not_found"
)

type Result struct {
	Verdict        Verdict
	UniversityCode string
	DiplomaNumber  string
	OwnerNameMask  string
	Program        string
}
