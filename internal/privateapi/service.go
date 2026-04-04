package privateapi

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	users  UserStore
	tokens *TokenManager
}

func NewService(users UserStore, tokens *TokenManager) *Service {
	return &Service{users: users, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, login, password string, role Role) (LoginResult, error) {
	login = strings.TrimSpace(login)
	password = strings.TrimSpace(password)
	if login == "" || password == "" || !role.Valid() {
		return LoginResult{}, NewAPIError(400, "Некорректные данные для входа")
	}

	user, found, err := s.users.FindByLoginAndRole(ctx, login, role)
	if err != nil {
		return LoginResult{}, fmt.Errorf("find auth user: %w", err)
	}
	if !found || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return LoginResult{}, NewAPIError(401, "Неверный логин или пароль")
	}

	token, err := s.tokens.Issue(user)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue jwt: %w", err)
	}

	return LoginResult{
		AccessToken: token,
		User: UserProfile{
			ID:               user.ID,
			Name:             user.Name,
			Role:             user.Role,
			OrganizationCode: user.OrganizationCode,
		},
	}, nil
}

func (s *Service) Me(ctx context.Context, claims Claims) (UserProfile, error) {
	user, found, err := s.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return UserProfile{}, fmt.Errorf("find auth user by id: %w", err)
	}
	if !found {
		return UserProfile{}, NewAPIError(401, "Невалидная сессия")
	}
	return UserProfile{
		ID:               user.ID,
		Name:             user.Name,
		Role:             user.Role,
		OrganizationCode: user.OrganizationCode,
	}, nil
}
