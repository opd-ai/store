// Package background provides background job functionality for the store.
package background

import (
	"context"
	"log"
	"time"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/pod"
	"github.com/opd-ai/store/pkg/store"
)

// PoDPoller polls print-on-demand order statuses periodically.
type PoDPoller struct {
	store    store.Service
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewPoDPoller creates a new PoD status poller.
func NewPoDPoller(store store.Service, interval time.Duration) *PoDPoller {
	return &PoDPoller{
		store:    store,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins the polling loop in a background goroutine.
func (p *PoDPoller) Start(ctx context.Context) {
	go p.run(ctx)
}

// Stop signals the poller to stop and waits for it to finish.
func (p *PoDPoller) Stop() {
	close(p.stopCh)
	<-p.doneCh
}

// run executes the polling loop.
func (p *PoDPoller) run(ctx context.Context) {
	defer close(p.doneCh)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Poll immediately on start
	p.pollOnce(ctx)

	for {
		select {
		case <-ticker.C:
			p.pollOnce(ctx)
		case <-p.stopCh:
			log.Println("PoD poller stopped")
			return
		case <-ctx.Done():
			log.Println("PoD poller context cancelled")
			return
		}
	}
}

// pollOnce performs a single polling cycle for all active PoD orders.
func (p *PoDPoller) pollOnce(ctx context.Context) {
	log.Println("Polling PoD order statuses...")

	// Get all fulfilled payments
	payments, err := p.store.ListPayments(ctx, map[string]interface{}{
		"status": "fulfilled",
	})
	if err != nil {
		log.Printf("Failed to list payments: %v", err)
		return
	}

	polledCount := 0
	updatedCount := 0

	for _, payment := range payments {
		// Check if this is a PoD order
		if !isPoDOrder(payment) {
			continue
		}

		polledCount++

		// Poll the provider for status
		if updated := p.pollOrder(ctx, payment); updated {
			updatedCount++
		}
	}

	log.Printf("PoD polling complete: %d orders polled, %d updated", polledCount, updatedCount)
}

// isPoDOrder checks if a payment is for a PoD item.
func isPoDOrder(payment *models.Payment) bool {
	if payment.FulfillmentResult == nil {
		return false
	}

	// Check if fulfillment result contains provider and order_id
	provider, hasProvider := payment.FulfillmentResult["provider"].(string)
	orderID, hasOrderID := payment.FulfillmentResult["order_id"].(string)

	return hasProvider && hasOrderID && provider != "" && orderID != ""
}

// pollOrder polls a single PoD order and updates its status.
func (p *PoDPoller) pollOrder(ctx context.Context, payment *models.Payment) bool {
	providerName, _ := payment.FulfillmentResult["provider"].(string)
	orderID, _ := payment.FulfillmentResult["order_id"].(string)

	// Get the item to retrieve API key
	item, err := p.store.GetItem(ctx, payment.ItemID)
	if err != nil {
		log.Printf("Failed to get item for payment %s: %v", payment.ID, err)
		return false
	}

	// Extract API key from backend config
	apiKey, ok := item.BackendConfig["api_key"].(string)
	if !ok || apiKey == "" {
		log.Printf("Missing API key for payment %s", payment.ID)
		return false
	}

	// Create provider instance
	provider, err := pod.NewProvider(providerName, apiKey)
	if err != nil {
		log.Printf("Failed to create provider for payment %s: %v", payment.ID, err)
		return false
	}

	// Get current status from provider
	status, err := provider.GetStatus(ctx, orderID)
	if err != nil {
		log.Printf("Failed to get status for order %s (payment %s): %v", orderID, payment.ID, err)
		return false
	}

	// Check if status has changed
	currentStatus, _ := payment.FulfillmentResult["status"].(string)
	if currentStatus == status.Status {
		// No change
		return false
	}

	// Update fulfillment result with new status
	updatedResult := models.JSONMap{}
	for k, v := range payment.FulfillmentResult {
		updatedResult[k] = v
	}
	updatedResult["status"] = status.Status
	updatedResult["tracking_url"] = status.TrackingURL
	updatedResult["shipping_date"] = status.ShippingDate
	updatedResult["last_updated"] = status.LastUpdated

	if err := p.store.UpdateFulfillmentResult(ctx, payment.ID, updatedResult); err != nil {
		log.Printf("Failed to update fulfillment result for payment %s: %v", payment.ID, err)
		return false
	}

	log.Printf("Updated status for order %s (payment %s): %s -> %s", orderID, payment.ID, currentStatus, status.Status)
	return true
}
