package store

import (
	"context"
	"log"
	"os"

	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
)

// PollPaymentStatus checks the paywall service for payment confirmation.
// Returns the remote invoice status if available, or nil if the payment
// is not pending or the paywall request fails.
func PollPaymentStatus(ctx context.Context, paywallClient paywall.Service, payment *models.Payment) *paywall.InvoiceStatus {
	// Only poll if payment has invoice ID and is pending
	if payment.InvoiceID == "" || payment.Status != "pending" {
		return nil
	}

	// Get status from paywall
	status, err := paywallClient.GetInvoiceStatus(ctx, payment.InvoiceID)
	if err != nil {
		log.Printf("Failed to get invoice status from paywall: %v", err)
		return nil
	}

	return status
}

// ConfirmAndFulfill marks a payment as confirmed and optionally triggers fulfillment.
// If autoFulfill is true, will attempt to fulfill the payment after confirmation.
// Returns an error if confirmation fails. Fulfillment errors are logged but not returned.
func ConfirmAndFulfill(ctx context.Context, storeService Service, payment *models.Payment, autoFulfill bool) error {
	// Skip if already confirmed
	if payment.Status == "confirmed" || payment.Status == "fulfilled" {
		return nil
	}

	// Confirm payment
	if err := storeService.ConfirmPayment(ctx, payment.ID, payment.InvoiceID); err != nil {
		log.Printf("Failed to confirm payment: %v", err)
		return err
	}

	// Auto-fulfill if enabled
	if autoFulfill {
		if err := storeService.FulfillPayment(ctx, payment.ID); err != nil {
			log.Printf("Auto-fulfillment failed for %s: %v", payment.ID, err)
			// Don't return error—payment is confirmed
		}
	}

	return nil
}

// ShouldAutoFulfill checks if auto-fulfillment is enabled via environment variable.
// Returns true if STORE_AUTO_FULFILL is empty or set to "true".
func ShouldAutoFulfill() bool {
	autoFulfill := os.Getenv("STORE_AUTO_FULFILL")
	return autoFulfill == "" || autoFulfill == "true"
}
