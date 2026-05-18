package background

import (
	"context"
	"testing"
	"time"

	"github.com/opd-ai/store/pkg/models"
)

func TestEscrowTimeoutChecker_CheckExpiredPayments(t *testing.T) {
	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	mockStore := &mockStoreService{
		payments: []*models.Payment{
			{
				ID:            "pay1",
				Status:        "confirmed",
				EscrowEnabled: true,
				EscrowState:   "funded",
				EscrowTimeout: &expiredTime,
			},
			{
				ID:            "pay2",
				Status:        "confirmed",
				EscrowEnabled: true,
				EscrowState:   "shipped",
				EscrowTimeout: &futureTime,
			},
			{
				ID:            "pay3",
				Status:        "confirmed",
				EscrowEnabled: false,
				EscrowTimeout: &expiredTime,
			},
			{
				ID:            "pay4",
				Status:        "confirmed",
				EscrowEnabled: true,
				EscrowState:   "released",
				EscrowTimeout: &expiredTime,
			},
		},
		escrowStateCalls: []escrowStateCall{},
	}

	checker := NewEscrowTimeoutChecker(mockStore, 1*time.Hour)
	checker.checkOnce(context.Background())

	if len(mockStore.escrowStateCalls) != 1 {
		t.Errorf("Expected 1 refund, got %d", len(mockStore.escrowStateCalls))
	}

	if len(mockStore.escrowStateCalls) > 0 && mockStore.escrowStateCalls[0].paymentID != "pay1" {
		t.Errorf("Expected to refund pay1, got %s", mockStore.escrowStateCalls[0].paymentID)
	}
}

func TestEscrowTimeoutChecker_StartStop(t *testing.T) {
	mockStore := &mockStoreService{
		payments:         []*models.Payment{},
		escrowStateCalls: []escrowStateCall{},
	}

	checker := NewEscrowTimeoutChecker(mockStore, 100*time.Millisecond)
	checker.Start(context.Background())

	time.Sleep(150 * time.Millisecond)

	checker.Stop()
}
