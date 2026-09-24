package service_test

import (
	"context"
	"testing"
	"time"

	"wacallerapi/internal/domain"
	"wacallerapi/internal/service"
)

type mockUserRepo struct {
	users map[string]*domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *domain.User) error {
	if m.users == nil {
		m.users = make(map[string]*domain.User)
	}
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) Update(ctx context.Context, u *domain.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	var list []*domain.User
	for _, u := range m.users {
		list = append(list, u)
	}
	return list, nil
}

func TestUserServiceRegister(t *testing.T) {
	repo := &mockUserRepo{}
	userSvc := service.NewUserService(repo, nil)

	ctx := context.Background()
	user, err := userSvc.Register(ctx, "test@example.com", "Test User", "Password123!", "developer")
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	if user.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %s", user.Email)
	}
	if user.PasswordVersion != 2 {
		t.Fatalf("expected password version 2, got %d", user.PasswordVersion)
	}

	// Duplicate registration should return ErrConflict
	_, err = userSvc.Register(ctx, "test@example.com", "Duplicate User", "Password123!", "developer")
	if err != domain.ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

type mockAuthRepo struct{}

func (m *mockAuthRepo) CreateIdentity(ctx context.Context, identity *domain.Identity) error { return nil }
func (m *mockAuthRepo) GetIdentityByProvider(ctx context.Context, provider, providerID string) (*domain.Identity, error) { return nil, domain.ErrNotFound }
func (m *mockAuthRepo) CreateSession(ctx context.Context, session *domain.AuthSession) error { return nil }
func (m *mockAuthRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.AuthSession, error) { return nil, domain.ErrNotFound }
func (m *mockAuthRepo) RevokeSession(ctx context.Context, id string) error { return nil }
func (m *mockAuthRepo) RevokeUserSessions(ctx context.Context, userID string) error { return nil }

type mockCredRepo struct {
	creds map[string]*domain.APICredential
}

func (m *mockCredRepo) Create(ctx context.Context, cred *domain.APICredential) error {
	if m.creds == nil {
		m.creds = make(map[string]*domain.APICredential)
	}
	m.creds[cred.ID] = cred
	m.creds[cred.KeyHash] = cred
	return nil
}

func (m *mockCredRepo) GetByHash(ctx context.Context, keyHash string) (*domain.APICredential, error) {
	if cred, ok := m.creds[keyHash]; ok {
		return cred, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockCredRepo) GetByID(ctx context.Context, id string) (*domain.APICredential, error) {
	if cred, ok := m.creds[id]; ok {
		return cred, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockCredRepo) ListByOrg(ctx context.Context, orgID string) ([]*domain.APICredential, error) { return nil, nil }
func (m *mockCredRepo) Revoke(ctx context.Context, id string) error { return nil }
func (m *mockCredRepo) UpdateLastUsed(ctx context.Context, id string, t time.Time) error { return nil }

func TestAuthServiceCredentials(t *testing.T) {
	userRepo := &mockUserRepo{}
	authRepo := &mockAuthRepo{}
	credRepo := &mockCredRepo{}

	authSvc := service.NewAuthService(userRepo, authRepo, credRepo, nil)
	ctx := context.Background()

	key, cred, err := authSvc.CreateAPICredential(ctx, "org_123", "Test Key", "usr_123", []string{"sessions:read"})
	if err != nil {
		t.Fatalf("failed to create api credential: %v", err)
	}

	if cred.OrganizationID != "org_123" {
		t.Fatalf("expected org_123, got %s", cred.OrganizationID)
	}

	validated, err := authSvc.ValidateAPICredential(ctx, key)
	if err != nil {
		t.Fatalf("failed to validate created key: %v", err)
	}
	if validated.ID != cred.ID {
		t.Fatalf("expected cred ID %s, got %s", cred.ID, validated.ID)
	}
}
