package service

import (
	"context"

	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type BillingService struct {
	subRepo repository.SubscriptionRepository
}

func NewBillingService(subRepo repository.SubscriptionRepository) *BillingService {
	return &BillingService{subRepo: subRepo}
}

func (s *BillingService) GetSubscription(ctx context.Context, orgID string) (*domain.Subscription, error) {
	return s.subRepo.GetByOrg(ctx, orgID)
}

func (s *BillingService) GetEntitlement(ctx context.Context, orgID, feature string) (int, error) {
	ent, err := s.subRepo.GetEntitlement(ctx, orgID, feature)
	if err != nil {
		// Default fallback limits if entitlement record doesn't exist yet
		switch feature {
		case "max_sessions":
			return 5, nil
		case "max_calls_per_day":
			return 1000, nil
		default:
			return 0, nil
		}
	}
	return ent.LimitValue, nil
}

func (s *BillingService) ListEntitlements(ctx context.Context, orgID string) ([]*domain.Entitlement, error) {
	return s.subRepo.ListEntitlements(ctx, orgID)
}
