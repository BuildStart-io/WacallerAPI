package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type UserService struct {
	userRepo repository.UserRepository
	audit    *AuditService
}

func NewUserService(userRepo repository.UserRepository, audit *AuditService) *UserService {
	return &UserService{
		userRepo: userRepo,
		audit:    audit,
	}
}

func (s *UserService) Register(ctx context.Context, email, name, password, role string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, domain.ErrInvalidInput
	}

	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil && existing != nil {
		return nil, domain.ErrConflict
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	userID := "usr_" + uuid.New().String()
	if role == "" {
		role = "developer"
	}

	user := &domain.User{
		ID:              userID,
		Email:           email,
		Name:            name,
		Role:            role,
		PasswordHash:    string(hashedPassword),
		PasswordVersion: 2,
		EmailVerified:   false,
		Status:          "active",
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, user.ID, "user", "", "user.register", "user", user.ID, "success", "", "", "", nil)
	}

	return user, nil
}

func (s *UserService) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *UserService) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}

func (s *UserService) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	return s.userRepo.List(ctx, limit, offset)
}
