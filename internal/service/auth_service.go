package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type AuthService struct {
	userRepo   repository.UserRepository
	authRepo   repository.AuthRepository
	credRepo   repository.CredentialRepository
	audit      *AuditService
}

func NewAuthService(
	userRepo repository.UserRepository,
	authRepo repository.AuthRepository,
	credRepo repository.CredentialRepository,
	audit *AuditService,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		authRepo:   authRepo,
		credRepo:   credRepo,
		audit:      audit,
	}
}

func (s *AuthService) AuthenticateUser(ctx context.Context, email, password string) (*domain.User, error) {
	user, err := s.userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if user.Status != "active" {
		return nil, domain.ErrForbidden
	}

	// Verify password (supports bcrypt or legacy SHA-256 with auto-rehash to bcrypt)
	if user.PasswordVersion == 1 {
		legacyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))
		if legacyHash != user.PasswordHash {
			return nil, domain.ErrUnauthorized
		}
		// Upgrade password to bcrypt v2 on successful login
		if newHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err == nil {
			user.PasswordHash = string(newHash)
			user.PasswordVersion = 2
			_ = s.userRepo.Update(ctx, user)
		}
	} else {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			return nil, domain.ErrUnauthorized
		}
	}

	return user, nil
}

func (s *AuthService) ValidateAPICredential(ctx context.Context, apiKey string) (*domain.APICredential, error) {
	if apiKey == "" {
		return nil, domain.ErrUnauthorized
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	cred, err := s.credRepo.GetByHash(ctx, keyHash)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if cred.RevokedAt != nil {
		return nil, domain.ErrUnauthorized
	}

	if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrUnauthorized
	}

	// Touch last used timestamp asynchronously/best-effort
	_ = s.credRepo.UpdateLastUsed(ctx, cred.ID, time.Now())

	return cred, nil
}

func (s *AuthService) CreateAPICredential(ctx context.Context, orgID, name, createdBy string, scopes []string) (string, *domain.APICredential, error) {
	rawKey := "wac_" + uuid.New().String() + uuid.New().String()
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	prefix := rawKey
	if len(rawKey) >= 8 {
		prefix = rawKey[:8]
	}

	credID := "cred_" + uuid.New().String()
	cred := &domain.APICredential{
		ID:             credID,
		OrganizationID: orgID,
		Name:           name,
		KeyHash:        keyHash,
		KeyPrefix:      prefix,
		Scopes:         scopes,
		CreatedBy:      createdBy,
	}

	if err := s.credRepo.Create(ctx, cred); err != nil {
		return "", nil, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, createdBy, "user", orgID, "credential.create", "api_credential", credID, "success", "", "", "", nil)
	}

	return rawKey, cred, nil
}
