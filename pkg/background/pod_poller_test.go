package background

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// mockStoreService is a mock implementation of store.Service for testing
type mockStoreService struct {
	payments       []*models.Payment
	items          map[string]*models.Item
	updateCalls    []updateCall
	listPaymentErr error
	getItemErr     error
	updateErr      error
}

type updateCall struct {
	paymentID string
	result    models.JSONMap
}

func (m *mockStoreService) ListPayments(ctx context.Context, filters map[string]interface{}) ([]*models.Payment, error) {
	if m.listPaymentErr != nil {
		return nil, m.listPaymentErr
	}

	// Apply filters
	statusFilter, hasStatus := filters["status"].(string)
	var result []*models.Payment
	for _, p := range m.payments {
		if hasStatus && p.Status != statusFilter {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

func (m *mockStoreService) GetItem(ctx context.Context, itemID string) (*models.Item, error) {
	if m.getItemErr != nil {
		return nil, m.getItemErr
	}
	item, ok := m.items[itemID]
	if !ok {
		return nil, fmt.Errorf("item not found")
	}
	return item, nil
}

func (m *mockStoreService) UpdateFulfillmentResult(ctx context.Context, paymentID string, result models.JSONMap) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, updateCall{paymentID: paymentID, result: result})
	return nil
}

// Stub implementations for other Service methods - not used in tests
func (m *mockStoreService) CreatePayment(ctx context.Context, itemID, amount, currency string) (*models.Payment, error) {
	return nil, nil
}
func (m *mockStoreService) UpdatePaymentInvoice(ctx context.Context, paymentID, invoiceID string) error {
	return nil
}
func (m *mockStoreService) UpdatePaymentPayerInfo(ctx context.Context, paymentID string, payerInfo models.JSONMap) error {
	return nil
}
func (m *mockStoreService) ConfirmPayment(ctx context.Context, paymentID, paymentHash string) error {
	return nil
}
func (m *mockStoreService) FulfillPayment(ctx context.Context, paymentID string) error { return nil }
func (m *mockStoreService) GetPayment(ctx context.Context, paymentID string) (*models.Payment, error) {
	return nil, nil
}
func (m *mockStoreService) GetPaymentByInvoiceID(ctx context.Context, invoiceID string) (*models.Payment, error) {
	return nil, nil
}
func (m *mockStoreService) GetCatalog(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockStoreService) SubmitFormData(ctx context.Context, paymentID string, formData models.JSONMap) (*models.FormSubmission, error) {
	return nil, nil
}
func (m *mockStoreService) GetFormSubmission(ctx context.Context, paymentID string) (*models.FormSubmission, error) {
	return nil, nil
}
func (m *mockStoreService) HandlerMetadata() map[string]handler.HandlerMetadata { return nil }
func (m *mockStoreService) CreateCategory(ctx context.Context, name, description string) (*models.Category, error) {
	return nil, nil
}
func (m *mockStoreService) ListCategories(ctx context.Context) ([]*models.Category, error) {
	return nil, nil
}
func (m *mockStoreService) UpdateCategory(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreService) DeleteCategory(ctx context.Context, id string) error { return nil }
func (m *mockStoreService) CreateItem(ctx context.Context, item *models.Item) (*models.Item, error) {
	return nil, nil
}
func (m *mockStoreService) ListItems(ctx context.Context, filters map[string]interface{}) ([]*models.Item, error) {
	return nil, nil
}
func (m *mockStoreService) UpdateItem(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreService) DeleteItem(ctx context.Context, id string) error { return nil }
func (m *mockStoreService) CreateTag(ctx context.Context, name string) (*models.Tag, error) {
	return nil, nil
}
func (m *mockStoreService) ListTags(ctx context.Context) ([]*models.Tag, error) { return nil, nil }
func (m *mockStoreService) UpdateTag(ctx context.Context, id string, updates map[string]interface{}) error {
	return nil
}
func (m *mockStoreService) DeleteTag(ctx context.Context, id string) error                { return nil }
func (m *mockStoreService) AddItemTag(ctx context.Context, itemID, tagID string) error    { return nil }
func (m *mockStoreService) RemoveItemTag(ctx context.Context, itemID, tagID string) error { return nil }
func (m *mockStoreService) GetItemTags(ctx context.Context, itemID string) ([]*models.Tag, error) {
	return nil, nil
}
func (m *mockStoreService) RecordDownload(ctx context.Context, paymentID, ipAddress, userAgent string) error {
	return nil
}
func (m *mockStoreService) GetDownloadCount(ctx context.Context, paymentID string) (int, error) {
	return 0, nil
}
func (m *mockStoreService) CheckDownloadLimit(ctx context.Context, paymentID string, maxDownloads int) (bool, error) {
	return true, nil
}
func (m *mockStoreService) CleanupOldAuditLogs(ctx context.Context, retentionDays int) (int, error) {
	return 0, nil
}

func TestNewPoDPoller(t *testing.T) {
	mockStore := &mockStoreService{}
	interval := 1 * time.Hour

	poller := NewPoDPoller(mockStore, interval)

	if poller == nil {
		t.Fatal("Expected poller to be created, got nil")
	}
	if poller.interval != interval {
		t.Errorf("Expected interval %v, got %v", interval, poller.interval)
	}
}

func TestIsPoDOrder(t *testing.T) {
	tests := []struct {
		name     string
		payment  *models.Payment
		expected bool
	}{
		{
			name: "Valid PoD order",
			payment: &models.Payment{
				FulfillmentResult: models.JSONMap{
					"provider": "printful",
					"order_id": "12345",
				},
			},
			expected: true,
		},
		{
			name: "Missing provider",
			payment: &models.Payment{
				FulfillmentResult: models.JSONMap{
					"order_id": "12345",
				},
			},
			expected: false,
		},
		{
			name: "Missing order_id",
			payment: &models.Payment{
				FulfillmentResult: models.JSONMap{
					"provider": "printful",
				},
			},
			expected: false,
		},
		{
			name: "Empty provider",
			payment: &models.Payment{
				FulfillmentResult: models.JSONMap{
					"provider": "",
					"order_id": "12345",
				},
			},
			expected: false,
		},
		{
			name:     "Nil fulfillment result",
			payment:  &models.Payment{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPoDOrder(tt.payment)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPollOnce_NoPayments(t *testing.T) {
	mockStore := &mockStoreService{
		payments: []*models.Payment{},
	}
	poller := NewPoDPoller(mockStore, 1*time.Hour)

	ctx := context.Background()
	poller.pollOnce(ctx)

	if len(mockStore.updateCalls) != 0 {
		t.Errorf("Expected no updates, got %d", len(mockStore.updateCalls))
	}
}

func TestPollOnce_NonPoDPayments(t *testing.T) {
	mockStore := &mockStoreService{
		payments: []*models.Payment{
			{
				ID:     "payment1",
				Status: "fulfilled",
				FulfillmentResult: models.JSONMap{
					"download_url": "http://example.com/file",
				},
			},
		},
	}
	poller := NewPoDPoller(mockStore, 1*time.Hour)

	ctx := context.Background()
	poller.pollOnce(ctx)

	if len(mockStore.updateCalls) != 0 {
		t.Errorf("Expected no updates for non-PoD payments, got %d", len(mockStore.updateCalls))
	}
}

func TestStartStop(t *testing.T) {
	mockStore := &mockStoreService{
		payments: []*models.Payment{},
	}
	poller := NewPoDPoller(mockStore, 100*time.Millisecond)

	ctx := context.Background()
	poller.Start(ctx)

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

	// Stop should complete quickly
	done := make(chan struct{})
	go func() {
		poller.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete in time")
	}
}
