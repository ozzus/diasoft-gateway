package privateapi

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

func (m *TokenManager) Issue(user User) (string, error) {
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":              user.Login,
		"userId":           user.ID,
		"name":             user.Name,
		"role":             string(user.Role),
		"organizationCode": user.OrganizationCode,
		"universityId":     user.UniversityID,
		"diplomaId":        user.DiplomaID,
		"iat":              now.Unix(),
		"exp":              now.Add(m.ttl).Unix(),
	})
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

func (m *TokenManager) Parse(tokenString string) (Claims, error) {
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("parse jwt: %w", err)
	}
	if !parsed.Valid {
		return Claims{}, fmt.Errorf("invalid jwt")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("unexpected jwt claims")
	}

	role := Role(fmt.Sprint(claims["role"]))
	if !role.Valid() {
		return Claims{}, fmt.Errorf("invalid role claim")
	}

	return Claims{
		UserID:           fmt.Sprint(claims["userId"]),
		Subject:          fmt.Sprint(claims["sub"]),
		Name:             fmt.Sprint(claims["name"]),
		Role:             role,
		OrganizationCode: stringifyClaim(claims["organizationCode"]),
		UniversityID:     stringifyClaim(claims["universityId"]),
		DiplomaID:        stringifyClaim(claims["diplomaId"]),
	}, nil
}

func stringifyClaim(value any) string {
	if value == nil {
		return ""
	}
	stringValue := fmt.Sprint(value)
	if stringValue == "<nil>" {
		return ""
	}
	return stringValue
}
