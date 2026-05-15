// Package main provides the store service executable.
// This is the main entry point for the opd-ai/store cryptocurrency payment gateway
// with pluggable fulfillment handlers for digital goods, physical shipping, and print-on-demand.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	bolt "go.etcd.io/bbolt"

	"github.com/opd-ai/store/internal/api"
	"github.com/opd-ai/store/internal/handlers"
	"github.com/opd-ai/store/pkg/background"
	"github.com/opd-ai/store/pkg/config"
	"github.com/opd-ai/store/pkg/crypto"
	"github.com/opd-ai/store/pkg/db"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/metrics"
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
)

func main() {
	// Parse command-line flags
	configFile := flag.String("config", "", "path to configuration file (optional)")
	flag.Parse()

	// Load .env file if present
	_ = godotenv.Load()

	// Load configuration (CLI flags > env vars > config file > defaults)
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Ensure required directories exist
	if err := ensureDirectories(); err != nil {
		log.Fatalf("Failed to create required directories: %v", err)
	}

	// Initialize database
	boltDB, err := initDatabase(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer boltDB.Close()

	// Initialize buckets
	if err := db.InitBuckets(boltDB); err != nil {
		log.Fatalf("Failed to initialize database buckets: %v", err)
	}

	// Initialize services
	apiHandler := initializeServices(boltDB, cfg)

	// Start background jobs
	var podPoller *background.PoDPoller
	if cfg.PoDPollingEnabled {
		podPoller = background.NewPoDPoller(apiHandler.Store(), cfg.PoDPollingInterval)
		podPoller.Start(context.Background())
		log.Printf("Started PoD polling with interval: %v", cfg.PoDPollingInterval)
	}

	// Setup router with all endpoints
	router := setupRouter(apiHandler, cfg)

	// Server configuration
	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	startServer(server, cfg.ServerPort)
	waitForShutdown(server, cfg.ShutdownTimeout, podPoller)
}

// initializeServices sets up the store service, paywall client, and API handler.
func initializeServices(boltDB *bolt.DB, cfg *config.Config) *api.Handler {
	// Initialize handler registry
	registry := handler.NewRegistry()
	if err := registerHandlers(registry); err != nil {
		log.Fatalf("Failed to register handlers: %v", err)
	}

	// Wrap BoltDB in Database interface
	database := db.NewBoltDatabase(boltDB)

	// Initialize store service
	storeService := store.NewStore(database, registry)

	// Initialize encryption service if enabled
	if cfg.EncryptionEnabled && cfg.EncryptionKey != "" {
		encryption, err := crypto.NewEncryptionServiceFromBase64(cfg.EncryptionKey)
		if err != nil {
			log.Fatalf("Failed to initialize encryption service: %v", err)
		}
		storeService.SetEncryption(encryption)
		log.Println("Encryption enabled for backend configurations")
	} else {
		log.Println("Warning: Encryption disabled or key not set, backend configurations will not be encrypted")
	}

	// Initialize paywall client
	paywallAPIKey := os.Getenv("STORE_PAYWALL_API_KEY")
	if cfg.PaywallURL == "" || paywallAPIKey == "" {
		log.Println("Warning: STORE_PAYWALL_URL or STORE_PAYWALL_API_KEY not set, paywall integration disabled")
	}
	paywallClient := paywall.NewClient(cfg.PaywallURL, paywallAPIKey)

	// Initialize API handlers
	apiHandler := api.NewHandler(storeService, paywallClient)

	return apiHandler
}

// setupRouter configures all routes and middleware.
func setupRouter(apiHandler *api.Handler, cfg *config.Config) *mux.Router {
	router := mux.NewRouter()

	// Public endpoints
	router.HandleFunc("/health", api.HealthHandler).Methods("GET")
	router.HandleFunc("/api/catalog", apiHandler.GetCatalog).Methods("GET")
	router.HandleFunc("/api/items/{id}", apiHandler.GetItem).Methods("GET")
	router.HandleFunc("/api/csrf-token", apiHandler.GetCSRFToken).Methods("GET")

	// Checkout endpoint with rate limiting if enabled
	if cfg.RateLimitEnabled {
		checkoutHandler := api.RateLimitMiddleware(cfg.RateLimitRPM, cfg.RateLimitBurst)(http.HandlerFunc(apiHandler.CreateCheckout))
		router.Handle("/api/checkout", checkoutHandler).Methods("POST")
	} else {
		router.HandleFunc("/api/checkout", apiHandler.CreateCheckout).Methods("POST")
	}

	router.HandleFunc("/api/payment/{id}/status", apiHandler.GetPaymentStatus).Methods("GET")

	// Form submission with CSRF protection if enabled
	if cfg.CSRFEnabled {
		formSubmitHandler := api.CSRFMiddleware(http.HandlerFunc(apiHandler.SubmitPaymentForm))
		router.Handle("/api/payment/{id}/submit-form", formSubmitHandler).Methods("POST")
	} else {
		router.HandleFunc("/api/payment/{id}/submit-form", apiHandler.SubmitPaymentForm).Methods("POST")
	}

	router.HandleFunc("/api/payment/{id}/download", apiHandler.TrackDownload).Methods("POST")

	// File download endpoint
	router.HandleFunc("/api/download/{payment_id}", apiHandler.ServeDownload).Methods("GET")

	// Webhook endpoints
	router.HandleFunc("/webhook/payment-confirmed", apiHandler.WebhookPaymentConfirmed).Methods("POST")

	// Admin endpoints
	router.HandleFunc("/admin/handlers", apiHandler.ListHandlers).Methods("GET")
	router.HandleFunc("/admin/payments", apiHandler.ListPayments).Methods("GET")
	router.HandleFunc("/admin/payment/{id}/confirm", apiHandler.ConfirmPayment).Methods("POST")
	router.HandleFunc("/admin/payment/{id}/fulfill", apiHandler.FulfillPayment).Methods("POST")
	router.HandleFunc("/admin/orders/{payment_id}/status", apiHandler.GetOrderStatus).Methods("GET")
	router.HandleFunc("/admin/audit-logs", apiHandler.ListAuditLogs).Methods("GET")

	// Register CRUD endpoints for resources
	registerCRUDEndpoints(router, apiHandler, "categories", apiHandler.CreateCategory, apiHandler.ListCategories, apiHandler.UpdateCategory, apiHandler.DeleteCategory)
	registerCRUDEndpoints(router, apiHandler, "items", apiHandler.CreateItem, apiHandler.ListItems, apiHandler.UpdateItem, apiHandler.DeleteItem)
	registerCRUDEndpoints(router, apiHandler, "tags", apiHandler.CreateTag, apiHandler.ListTags, apiHandler.UpdateTag, apiHandler.DeleteTag)

	// Item-Tag association endpoints
	router.HandleFunc("/admin/items/{id}/tags", apiHandler.AddItemTag).Methods("POST")
	router.HandleFunc("/admin/items/{id}/tags/{tag_id}", apiHandler.RemoveItemTag).Methods("DELETE")

	// API documentation endpoints
	docsDir := "./docs/api"
	router.PathPrefix("/api/docs").Handler(http.StripPrefix("/api/docs", http.FileServer(http.Dir(docsDir)))).Methods("GET")

	// Prometheus metrics endpoint
	router.Handle("/metrics", promhttp.Handler()).Methods("GET")

	// Middleware
	router.Use(metrics.HTTPMiddleware) // Metrics collection
	router.Use(api.CORSMiddleware)
	router.Use(api.LoggingMiddleware)

	return router
}

// startServer starts the HTTP server in a goroutine.
func startServer(server *http.Server, port string) {
	go func() {
		log.Printf("Starting server on port %s\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()
}

// waitForShutdown waits for interrupt signal and performs graceful shutdown.
func waitForShutdown(server *http.Server, shutdownTimeout time.Duration, podPoller *background.PoDPoller) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	// Stop background jobs first
	if podPoller != nil {
		log.Println("Stopping PoD poller...")
		podPoller.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// ensureDirectories creates required directories if they don't exist.
func ensureDirectories() error {
	dirs := []string{}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		log.Printf("Ensured directory exists: %s", dir)
	}

	return nil
}

// initDatabase initializes the database connection.
func initDatabase(dbPath string) (*bolt.DB, error) {
	// Ensure the data directory exists
	if err := os.MkdirAll("./data", 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	boltDB, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return boltDB, nil
}

// registerHandlers registers all fulfillment handlers.
func registerHandlers(registry handler.HandlerRegistry) error {
	handlersToRegister := []handler.FulfillmentHandler{
		handlers.NewDigitalMediaHandler(),
		handlers.NewShippingFormHandler(),
		handlers.NewPrintOnDemandHandler(),
		handlers.NewCustomHandler(),
	}

	for _, h := range handlersToRegister {
		if err := registry.Register(h); err != nil {
			return fmt.Errorf("failed to register handler: %w", err)
		}
	}

	return nil
}

// registerCRUDEndpoints registers standard CRUD routes for a resource.
func registerCRUDEndpoints(router *mux.Router, apiHandler *api.Handler, resource string, create, list, update, delete func(http.ResponseWriter, *http.Request)) {
	basePath := "/admin/" + resource
	idPath := basePath + "/{id}"

	router.HandleFunc(basePath, create).Methods("POST")
	router.HandleFunc(basePath, list).Methods("GET")
	router.HandleFunc(idPath, update).Methods("PUT")
	router.HandleFunc(idPath, delete).Methods("DELETE")
}
