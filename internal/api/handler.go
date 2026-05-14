package api

import (
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
)

// Handler encapsulates HTTP handlers for the store API.
type Handler struct {
	store         store.Service
	paywallClient paywall.Service
}

// NewHandler creates a new API handler.
func NewHandler(s store.Service, paywallClient paywall.Service) *Handler {
	return &Handler{
		store:         s,
		paywallClient: paywallClient,
	}
}
