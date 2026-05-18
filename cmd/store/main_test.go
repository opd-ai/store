package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/opd-ai/store/pkg/config"
	"github.com/opd-ai/store/pkg/handler"
)

// testConfig creates a Config with sensible defaults for testing
func testConfig() *config.Config {
	return &config.Config{
		ServerPort:              "8080",
		ReadTimeout:             10 * time.Second,
		WriteTimeout:            10 * time.Second,
		ShutdownTimeout:         30 * time.Second,
		DatabasePath:            "./data/store.db",
		PaywallTestnet:          true,
		PaywallDBPath:           "./data/paywall.db",
		PaywallTimeout:          24 * time.Hour,
		PaywallMinConfirmations: 1,
		EncryptionEnabled:       false,
		EncryptionKey:           "",
		RateLimitEnabled:        true,
		RateLimitRPM:            60,
		RateLimitBurst:          10,
		CSRFEnabled:             true,
		AdminToken:              "",
		CORSOrigins:             []string{"*"},
	}
}

func TestInitDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := initDatabase(dbPath)
	if err != nil {
		t.Fatalf("initDatabase failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("expected non-nil database")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file was not created at %s", dbPath)
	}
}

func TestInitDatabase_DefaultPath(t *testing.T) {
	testDBPath := "./data/store.db"
	defer os.RemoveAll("./data")

	db, err := initDatabase(testDBPath)
	if err != nil {
		t.Fatalf("initDatabase with default path failed: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(testDBPath); os.IsNotExist(err) {
		t.Error("database was not created at default path")
	}
}

func TestRegisterHandlers(t *testing.T) {
	registry := handler.NewRegistry()

	err := registerHandlers(registry)
	if err != nil {
		t.Fatalf("registerHandlers failed: %v", err)
	}

	expectedTypes := []string{"digital_media", "shipping_form", "pod", "custom"}
	for _, handlerType := range expectedTypes {
		h, err := registry.Get(handlerType)
		if err != nil {
			t.Errorf("handler type %s not registered: %v", handlerType, err)
		}
		if h == nil {
			t.Errorf("handler type %s is nil", handlerType)
		}
	}
}

func TestRegisterHandlers_Empty(t *testing.T) {
	registry := handler.NewRegistry()

	handlers := registry.All()
	if len(handlers) != 0 {
		t.Errorf("expected empty registry, got %d handlers", len(handlers))
	}

	if err := registerHandlers(registry); err != nil {
		t.Fatalf("registerHandlers failed: %v", err)
	}

	handlers = registry.All()
}

func TestSetupRouter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	apiHandler := initializeServices(boltDB, testConfig())
	router := setupRouter(apiHandler, testConfig())

	if router == nil {
		t.Fatal("setupRouter returned nil router")
	}

	testRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/api/catalog"},
		{"GET", "/api/items/test-id"},
		{"POST", "/api/checkout"},
		{"GET", "/api/payment/test-id/status"},
		{"POST", "/admin/categories"},
		{"GET", "/admin/categories"},
	}

	for _, tc := range testRoutes {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code == 404 {
			t.Logf("Note: %s %s returned 404", tc.method, tc.path)
		}
	}
}

func TestInitializeServices(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	apiHandler := initializeServices(boltDB, testConfig())

	if apiHandler == nil {
		t.Fatal("initializeServices returned nil handler")
	}
}

func TestInitializeServices_WithEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	originalKey := os.Getenv("STORE_ENCRYPTION_KEY")
	defer os.Setenv("STORE_ENCRYPTION_KEY", originalKey)
	os.Setenv("STORE_ENCRYPTION_KEY", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")

	apiHandler := initializeServices(boltDB, testConfig())

	if apiHandler == nil {
		t.Fatal("initializeServices returned nil handler")
	}
}

func TestInitializeServices_MissingPaywallConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	originalURL := os.Getenv("STORE_PAYWALL_URL")
	originalKey := os.Getenv("STORE_PAYWALL_API_KEY")
	defer func() {
		os.Setenv("STORE_PAYWALL_URL", originalURL)
		os.Setenv("STORE_PAYWALL_API_KEY", originalKey)
	}()
	os.Unsetenv("STORE_PAYWALL_URL")
	os.Unsetenv("STORE_PAYWALL_API_KEY")

	apiHandler := initializeServices(boltDB, testConfig())

	if apiHandler == nil {
		t.Fatal("initializeServices returned nil handler")
	}
}

func TestEnsureDirectories(t *testing.T) {
	err := ensureDirectories()
	if err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}
}

func TestEnsureDirectories_WithDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test", "nested")

	dirs := []string{testDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Errorf("directory was not created: %s", testDir)
	}
}

func TestRegisterCRUDEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	apiHandler := initializeServices(boltDB, testConfig())
	router := setupRouter(apiHandler, testConfig())

	crudTests := []struct {
		method string
		path   string
	}{
		{"POST", "/admin/categories"},
		{"GET", "/admin/categories"},
		{"PUT", "/admin/categories/123"},
		{"DELETE", "/admin/categories/123"},
	}

	for _, tc := range crudTests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-Admin-Token", "test-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code == 404 {
			t.Errorf("%s %s returned 404, route may not be registered", tc.method, tc.path)
		}
	}
}

func TestStartServer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer boltDB.Close()

	apiHandler := initializeServices(boltDB, testConfig())
	router := setupRouter(apiHandler, testConfig())

	server := &http.Server{
		Addr:         ":0",
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	time.Sleep(100 * time.Millisecond)

	if err := server.Close(); err != nil {
		t.Errorf("failed to close server: %v", err)
	}
}

func TestInitEmbeddedPaywall(t *testing.T) {
	cfg := testConfig()
	cfg.MultisigEnabled = false

	svc, err := initEmbeddedPaywall(cfg)
	if err != nil {
		t.Fatalf("initEmbeddedPaywall failed: %v", err)
	}

	if svc == nil {
		t.Fatal("expected non-nil paywall service")
	}
}

func TestInitDatabase_Error(t *testing.T) {
	_, err := initDatabase("/invalid/path/to/db.db")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
