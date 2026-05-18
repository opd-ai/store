package background

import (
	"context"
	"testing"
	"time"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// mockStoreForAuditCleaner is a minimal mock implementation of store.Service for testing.
type mockStoreForAuditCleaner struct {
	cleanupOldAuditLogsFunc func(ctx context.Context, retentionDays int) (int, error)
}

func (m *mockStoreForAuditCleaner) CleanupOldAuditLogs(ctx context.Context, retentionDays int) (int, error) {
	if m.cleanupOldAuditLogsFunc != nil {
		return m.cleanupOldAuditLogsFunc(ctx, retentionDays)
	}
	return 0, nil
}

// Minimal stub implementations of other Service methods required by the interface
func (m *mockStoreForAuditCleaner) CreateCategory(ctx context.Context, name, description string) (*models.Category, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) ListCategories(ctx context.Context) ([]*models.Category, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreForAuditCleaner) DeleteCategory(ctx context.Context, id string) error { return nil }
func (m *mockStoreForAuditCleaner) CreateTag(ctx context.Context, name string) (*models.Tag, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) ListTags(ctx context.Context) ([]*models.Tag, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreForAuditCleaner) DeleteTag(ctx context.Context, id string) error { return nil }
func (m *mockStoreForAuditCleaner) AddItemTag(ctx context.Context, itemID, tagID string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) RemoveItemTag(ctx context.Context, itemID, tagID string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) GetItemTags(ctx context.Context, itemID string) ([]*models.Tag, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) CreateItem(ctx context.Context, item *models.Item) (*models.Item, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) GetItem(ctx context.Context, id string) (*models.Item, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) ListItems(ctx context.Context, filters map[string]interface{}) ([]*models.Item, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreForAuditCleaner) DeleteItem(ctx context.Context, id string) error { return nil }
func (m *mockStoreForAuditCleaner) CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) GetPayment(ctx context.Context, id string) (*models.Payment, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdatePaymentEscrow(ctx context.Context, paymentID string, escrowEnabled bool, escrowState string, escrowTimeout *time.Time) error {
	return nil
}

func (m *mockStoreForAuditCleaner) GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) FulfillPayment(ctx context.Context, paymentID string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) GetDownloadCount(ctx context.Context, paymentID string) (int, error) {
	return 0, nil
}

func (m *mockStoreForAuditCleaner) CheckDownloadLimit(ctx context.Context, paymentID string, maxDownloads int) (bool, error) {
	return false, nil
}

func (m *mockStoreForAuditCleaner) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	return nil
}

func (m *mockStoreForAuditCleaner) ListAuditLogs(ctx context.Context, filters map[string]interface{}) ([]*models.AuditLog, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) UpdateEscrowState(ctx context.Context, paymentID, newState string, additionalData models.JSONMap) error {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdateEscrowSignatures(ctx context.Context, paymentID string, signatures []models.EscrowSignature) error {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdateEscrowDispute(ctx context.Context, paymentID, reason string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdateEscrowResolution(ctx context.Context, paymentID, resolution string) error {
	return nil
}

func (m *mockStoreForAuditCleaner) GetCatalog(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}

func (m *mockStoreForAuditCleaner) HandlerMetadata() map[string]handler.HandlerMetadata {
	return nil
}

func (m *mockStoreForAuditCleaner) UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error {
	return nil
}

// TestAuditLogCleaner_CleanupOldLogs tests the cleanup functionality.
func TestAuditLogCleaner_CleanupOldLogs(t *testing.T) {
	deletedCount := 0
	mock := &mockStoreForAuditCleaner{
		cleanupOldAuditLogsFunc: func(ctx context.Context, retentionDays int) (int, error) {
			if retentionDays != 90 {
				t.Errorf("Expected retentionDays=90, got %d", retentionDays)
			}
			deletedCount++
			return 5, nil
		},
	}

	cleaner := NewAuditLogCleaner(mock, 100*time.Millisecond, 90)
	ctx := context.Background()

	cleaner.Start(ctx)
	time.Sleep(250 * time.Millisecond) // Wait for at least 2 cleanup cycles
	cleaner.Stop()

	if deletedCount < 2 {
		t.Errorf("Expected at least 2 cleanup cycles, got %d", deletedCount)
	}
}

// TestAuditLogCleaner_ZeroRetention tests that cleanup is skipped when retention is 0.
func TestAuditLogCleaner_ZeroRetention(t *testing.T) {
	callCount := 0
	mock := &mockStoreForAuditCleaner{
		cleanupOldAuditLogsFunc: func(ctx context.Context, retentionDays int) (int, error) {
			callCount++
			return 0, nil
		},
	}

	cleaner := NewAuditLogCleaner(mock, 100*time.Millisecond, 0)
	ctx := context.Background()

	cleaner.Start(ctx)
	time.Sleep(250 * time.Millisecond)
	cleaner.Stop()

	if callCount > 0 {
		t.Errorf("Expected no cleanup calls with zero retention, got %d", callCount)
	}
}

// TestAuditLogCleaner_StopImmediately tests immediate shutdown.
func TestAuditLogCleaner_StopImmediately(t *testing.T) {
	mock := &mockStoreForAuditCleaner{}
	cleaner := NewAuditLogCleaner(mock, 1*time.Second, 90)
	ctx := context.Background()

	cleaner.Start(ctx)
	cleaner.Stop()
	// Should not panic or hang
}
