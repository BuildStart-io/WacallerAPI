package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type OrgService struct {
	orgRepo        repository.OrgRepository
	membershipRepo repository.MembershipRepository
	audit          *AuditService
}

func NewOrgService(orgRepo repository.OrgRepository, membershipRepo repository.MembershipRepository, audit *AuditService) *OrgService {
	return &OrgService{
		orgRepo:        orgRepo,
		membershipRepo: membershipRepo,
		audit:          audit,
	}
}

func (s *OrgService) CreateOrg(ctx context.Context, ownerUserID, name, slug string) (*domain.Organization, *domain.Membership, error) {
	if name == "" || slug == "" {
		return nil, nil, domain.ErrInvalidInput
	}

	orgID := "org_" + uuid.New().String()
	org := &domain.Organization{
		ID:     orgID,
		Name:   name,
		Slug:   slug,
		Status: "active",
	}

	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, nil, err
	}

	memID := "mem_" + uuid.New().String()
	mem := &domain.Membership{
		ID:             memID,
		UserID:         ownerUserID,
		OrganizationID: orgID,
		Role:           "owner",
	}

	if err := s.membershipRepo.Create(ctx, mem); err != nil {
		return nil, nil, fmt.Errorf("create owner membership: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, ownerUserID, "user", orgID, "org.create", "organization", orgID, "success", "", "", "", nil)
	}

	return org, mem, nil
}

func (s *OrgService) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	return s.orgRepo.GetByID(ctx, id)
}

func (s *OrgService) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	return s.orgRepo.GetBySlug(ctx, slug)
}

func (s *OrgService) AddMember(ctx context.Context, actorID, orgID, targetUserID, role string) (*domain.Membership, error) {
	memID := "mem_" + uuid.New().String()
	mem := &domain.Membership{
		ID:             memID,
		UserID:         targetUserID,
		OrganizationID: orgID,
		Role:           role,
	}

	if err := s.membershipRepo.Create(ctx, mem); err != nil {
		return nil, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "user", orgID, "member.add", "user", targetUserID, "success", "", "", "", map[string]any{"role": role})
	}

	return mem, nil
}

func (s *OrgService) RemoveMember(ctx context.Context, actorID, membershipID string) error {
	mem, err := s.membershipRepo.GetByID(ctx, membershipID)
	if err != nil {
		return err
	}

	if err := s.membershipRepo.Delete(ctx, membershipID); err != nil {
		return err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "user", mem.OrganizationID, "member.remove", "user", mem.UserID, "success", "", "", "", nil)
	}

	return nil
}

func (s *OrgService) ListMembers(ctx context.Context, orgID string) ([]*domain.Membership, error) {
	return s.membershipRepo.ListByOrg(ctx, orgID)
}

func (s *OrgService) GetUserMemberships(ctx context.Context, userID string) ([]*domain.Membership, error) {
	return s.membershipRepo.ListByUser(ctx, userID)
}
