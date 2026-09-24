package domain_test

import (
	"testing"
	"wacallerapi/internal/domain"
)

func TestDomainErrors(t *testing.T) {
	if domain.ErrNotFound.Error() != "resource not found" {
		t.Fatalf("unexpected ErrNotFound message: %s", domain.ErrNotFound.Error())
	}
	if domain.ErrUnauthorized.Error() != "unauthorized" {
		t.Fatalf("unexpected ErrUnauthorized message: %s", domain.ErrUnauthorized.Error())
	}
}

func TestDomainEvents(t *testing.T) {
	if domain.EventOrgCreated != "org.created" {
		t.Fatalf("unexpected event string: %s", domain.EventOrgCreated)
	}
	if domain.EventCallCreated != "call.created" {
		t.Fatalf("unexpected event string: %s", domain.EventCallCreated)
	}
}
