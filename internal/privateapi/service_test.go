package privateapi

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type userStoreStub struct {
	byLogin map[string]User
	byID    map[string]User
}

func (s userStoreStub) FindByLoginAndRole(_ context.Context, login string, role Role) (User, bool, error) {
	lookup, ok := s.byLogin[string(role)+":"+login]
	return lookup, ok, nil
}

func (s userStoreStub) FindByID(_ context.Context, id string) (User, bool, error) {
	lookup, ok := s.byID[id]
	return lookup, ok, nil
}

func TestServiceLoginIssuesTokenForValidCredentials(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate password hash: %v", err)
	}

	user := User{
		ID:               "user-1",
		Login:            "ITMO",
		PasswordHash:     string(passwordHash),
		Name:             "ITMO Operator",
		Role:             RoleUniversity,
		OrganizationCode: "ITMO",
		UniversityID:     "11111111-1111-1111-1111-111111111111",
	}

	service := NewService(
		userStoreStub{
			byLogin: map[string]User{"university:ITMO": user},
			byID:    map[string]User{"user-1": user},
		},
		NewTokenManager("test-secret", time.Hour),
	)

	result, err := service.Login(context.Background(), "ITMO", "secret", RoleUniversity)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("expected access token to be issued")
	}
	if result.User.Role != RoleUniversity {
		t.Fatalf("user role = %s, want %s", result.User.Role, RoleUniversity)
	}
	if result.User.OrganizationCode != "ITMO" {
		t.Fatalf("organization code = %s, want ITMO", result.User.OrganizationCode)
	}
}

func TestServiceLoginRejectsInvalidCredentials(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate password hash: %v", err)
	}

	service := NewService(
		userStoreStub{
			byLogin: map[string]User{
				"student:D-2026-0001": {
					ID:           "student-1",
					Login:        "D-2026-0001",
					PasswordHash: string(passwordHash),
					Name:         "Иван Иванов",
					Role:         RoleStudent,
					DiplomaID:    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				},
			},
		},
		NewTokenManager("test-secret", time.Hour),
	)

	_, err = service.Login(context.Background(), "D-2026-0001", "wrong-password", RoleStudent)
	if err == nil {
		t.Fatal("expected login error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Fatalf("status code = %d, want 401", apiErr.StatusCode)
	}
}

func TestTokenManagerRoundTrip(t *testing.T) {
	manager := NewTokenManager("test-secret", time.Hour)
	user := User{
		ID:               "user-1",
		Login:            "hr@diplomverify.ru",
		Name:             "HR Demo",
		Role:             RoleHR,
		OrganizationCode: "",
	}

	token, err := manager.Issue(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("claims userId = %s, want user-1", claims.UserID)
	}
	if claims.Role != RoleHR {
		t.Fatalf("claims role = %s, want %s", claims.Role, RoleHR)
	}
	if claims.Subject != "hr@diplomverify.ru" {
		t.Fatalf("claims sub = %s, want hr@diplomverify.ru", claims.Subject)
	}
}
