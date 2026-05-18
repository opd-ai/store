package background

import (
	"context"
	"log"
	"time"

	"github.com/opd-ai/store/pkg/store"
)

// EscrowTimeoutChecker checks for expired escrow payments and auto-refunds them.
type EscrowTimeoutChecker struct {
	store    store.Service
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewEscrowTimeoutChecker creates a new escrow timeout checker.
func NewEscrowTimeoutChecker(store store.Service, interval time.Duration) *EscrowTimeoutChecker {
	return &EscrowTimeoutChecker{
		store:    store,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins the checking loop in a background goroutine.
func (c *EscrowTimeoutChecker) Start(ctx context.Context) {
	go c.run(ctx)
}

// Stop signals the checker to stop and waits for it to finish.
func (c *EscrowTimeoutChecker) Stop() {
	close(c.stopCh)
	<-c.doneCh
}

// run executes the checking loop.
func (c *EscrowTimeoutChecker) run(ctx context.Context) {
	defer close(c.doneCh)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Check immediately on start
	c.checkOnce(ctx)

	for {
		select {
		case <-ticker.C:
			c.checkOnce(ctx)
		case <-c.stopCh:
			log.Println("Escrow timeout checker stopped")
			return
		case <-ctx.Done():
			log.Println("Escrow timeout checker context cancelled")
			return
		}
	}
}

// checkOnce performs a single check cycle for expired escrow payments.
func (c *EscrowTimeoutChecker) checkOnce(ctx context.Context) {
	log.Println("Checking for expired escrow payments...")

	// Get all confirmed payments with escrow enabled
	payments, err := c.store.ListPayments(ctx, map[string]interface{}{
		"status": "confirmed",
	})
	if err != nil {
		log.Printf("Failed to list payments: %v", err)
		return
	}

	checkedCount := 0
	refundedCount := 0

	now := time.Now()
	for _, payment := range payments {
		// Skip non-escrow payments
		if !payment.EscrowEnabled {
			continue
		}

		// Skip if escrow already released or refunded
		if payment.EscrowState == "released" || payment.EscrowState == "refunded" {
			continue
		}

		// Skip if no timeout set
		if payment.EscrowTimeout == nil {
			continue
		}

		checkedCount++

		// Check if timeout has expired
		if payment.EscrowTimeout.Before(now) {
			log.Printf("Escrow timeout expired for payment %s (state: %s, timeout: %v)",
				payment.ID, payment.EscrowState, payment.EscrowTimeout)

			// Auto-refund expired escrow
			if err := c.store.UpdateEscrowState(ctx, payment.ID, "refunded", nil); err != nil {
				log.Printf("Failed to refund expired escrow payment %s: %v", payment.ID, err)
				continue
			}

			refundedCount++
			log.Printf("Auto-refunded expired escrow payment %s", payment.ID)
		}
	}

	if checkedCount > 0 {
		log.Printf("Escrow timeout check complete: %d checked, %d refunded", checkedCount, refundedCount)
	}
}
