package privateapi

import (
	"context"
	"time"
)

type Role string

const (
	RoleUniversity Role = "university"
	RoleStudent    Role = "student"
	RoleHR         Role = "hr"
)

func (r Role) Valid() bool {
	switch r {
	case RoleUniversity, RoleStudent, RoleHR:
		return true
	default:
		return false
	}
}

type User struct {
	ID               string
	Login            string
	PasswordHash     string
	Name             string
	Role             Role
	OrganizationCode string
	UniversityID     string
	DiplomaID        string
}

type Claims struct {
	UserID           string
	Subject          string
	Name             string
	Role             Role
	OrganizationCode string
	UniversityID     string
	DiplomaID        string
}

type UserProfile struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Role             Role   `json:"role"`
	OrganizationCode string `json:"organizationCode,omitempty"`
}

type LoginResult struct {
	AccessToken string      `json:"accessToken"`
	User        UserProfile `json:"user"`
}

type UniversityDiplomaItem struct {
	ID                string     `json:"id"`
	DiplomaNumber     string     `json:"diplomaNumber"`
	UniversityCode    string     `json:"universityCode"`
	OwnerName         string     `json:"ownerName"`
	OwnerNameMask     string     `json:"ownerNameMask"`
	Program           string     `json:"program"`
	GraduationYear    *int       `json:"graduationYear,omitempty"`
	Status            string     `json:"status"`
	Hash              string     `json:"hash,omitempty"`
	VerificationToken string     `json:"verificationToken,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	RevokedAt         *time.Time `json:"revokedAt,omitempty"`
	RevokeReason      string     `json:"revokeReason,omitempty"`
}

type UniversityDiplomaList struct {
	Items []UniversityDiplomaItem `json:"items"`
	Total int64                   `json:"total"`
}

type ImportAccepted struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

type ImportStatus struct {
	JobID     string    `json:"jobId"`
	Status    string    `json:"status"`
	Total     *int      `json:"total,omitempty"`
	Imported  int       `json:"imported"`
	Failed    int       `json:"failed"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type StudentDiploma struct {
	ID                string `json:"id"`
	DiplomaNumber     string `json:"diplomaNumber"`
	UniversityCode    string `json:"universityCode"`
	Program           string `json:"program"`
	GraduationYear    *int   `json:"graduationYear,omitempty"`
	Status            string `json:"status"`
	VerificationToken string `json:"verificationToken"`
}

type ShareLink struct {
	ShareToken string    `json:"shareToken"`
	ShareURL   string    `json:"shareUrl"`
	ExpiresAt  time.Time `json:"expiresAt"`
	TTLSeconds int       `json:"ttlSeconds"`
}

type QRResponse struct {
	VerificationToken string    `json:"verificationToken"`
	QRURL             string    `json:"qrUrl"`
	ExpiresAt         time.Time `json:"expiresAt"`
}

type UserStore interface {
	FindByLoginAndRole(ctx context.Context, login string, role Role) (User, bool, error)
	FindByID(ctx context.Context, id string) (User, bool, error)
}

type RegistryClient interface {
	ListUniversityDiplomas(ctx context.Context, universityID, search, status string, page int) (UniversityDiplomaList, error)
	UploadUniversityDiplomas(ctx context.Context, universityID, filename, contentType string, content []byte) (ImportAccepted, error)
	GetUniversityImport(ctx context.Context, universityID, jobID string) (ImportStatus, error)
	GetUniversityImportErrors(ctx context.Context, universityID, jobID string) ([]ImportError, error)
	RevokeUniversityDiploma(ctx context.Context, universityID, diplomaID, reason string) error
	GetUniversityQR(ctx context.Context, universityID, diplomaID string) (QRResponse, error)
	GetStudentDiploma(ctx context.Context, diplomaID string) (StudentDiploma, error)
	CreateStudentShareLink(ctx context.Context, diplomaID string, ttlSeconds int) (ShareLink, error)
	DeleteStudentShareLink(ctx context.Context, diplomaID, token string) error
}
