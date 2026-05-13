package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/opd-ai/store/internal/api"
	"github.com/opd-ai/store/internal/handlers"
	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
	"github.com/opd-ai/store/pkg/paywall"
	"github.com/opd-ai/store/pkg/store"
)

func main() {
	// Load .env file if present
	_ = godotenv.Load()

	// Ensure required directories exist
	if err := ensureDirectories(); err != nil {
		log.Fatalf("Failed to create required directories: %v", err)
	}

	// Initialize database
	db, err := initDatabase()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Auto-migrate models
	if err := db.AutoMigrate(
		&models.Category{},
		&models.Tag{},
		&models.Item{},
		&models.Payment{},
		&models.FormSubmission{},
		&models.DownloadLog{},
	); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize services
	apiHandler := initializeServices(db)

	// Setup router with all endpoints
	router := setupRouter(apiHandler)

	// Server configuration
	port := os.Getenv("STORE_PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	startServer(server, port)
	waitForShutdown(server)
}

// initializeServices sets up the store service, paywall client, and API handler.
func initializeServices(db *gorm.DB) *api.Handler {
	// Initialize handler registry
	registry := handler.NewRegistry()
	if err := registerHandlers(registry); err != nil {
		log.Fatalf("Failed to register handlers: %v", err)
	}

	// Initialize store service
	storeService := store.NewStore(db, registry)

	// Initialize paywall client
	paywallURL := os.Getenv("STORE_PAYWALL_URL")
	paywallAPIKey := os.Getenv("STORE_PAYWALL_API_KEY")
	if paywallURL == "" || paywallAPIKey == "" {
		log.Println("Warning: STORE_PAYWALL_URL or STORE_PAYWALL_API_KEY not set, paywall integration disabled")
	}
	paywallClient := paywall.NewClient(paywallURL, paywallAPIKey)

	// Initialize API handlers
	apiHandler := api.NewHandler(storeService, paywallClient)

	return apiHandler
}

// setupRouter configures all routes and middleware.
func setupRouter(apiHandler *api.Handler) *mux.Router {
	router := mux.NewRouter()

	// Public endpoints
	router.HandleFunc("/health", api.HealthHandler).Methods("GET")
	router.HandleFunc("/api/catalog", apiHandler.GetCatalog).Methods("GET")
	router.HandleFunc("/api/items/{id}", apiHandler.GetItem).Methods("GET")
	router.HandleFunc("/api/checkout", apiHandler.CreateCheckout).Methods("POST")
	router.HandleFunc("/api/payment/{id}/status", apiHandler.GetPaymentStatus).Methods("GET")
	router.HandleFunc("/api/payment/{id}/submit-form", apiHandler.SubmitPaymentForm).Methods("POST")
	router.HandleFunc("/api/payment/{id}/download", apiHandler.TrackDownload).Methods("POST")

	// Webhook endpoints
	router.HandleFunc("/webhook/payment-confirmed", apiHandler.WebhookPaymentConfirmed).Methods("POST")

	// Admin endpoints
	router.HandleFunc("/admin/handlers", apiHandler.ListHandlers).Methods("GET")
	router.HandleFunc("/admin/payments", apiHandler.ListPayments).Methods("GET")
	router.HandleFunc("/admin/payment/{id}/confirm", apiHandler.ConfirmPayment).Methods("POST")
	router.HandleFunc("/admin/payment/{id}/fulfill", apiHandler.FulfillPayment).Methods("POST")
	router.HandleFunc("/admin/orders/{payment_id}/status", apiHandler.GetOrderStatus).Methods("GET")

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

	// Middleware
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
func waitForShutdown(server *http.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// ensureDirectories creates required directories if they don't exist.
func ensureDirectories() error {
	dirs := []string{
		os.Getenv("STORE_UPLOADS_DIR"),
		os.Getenv("STORE_TEMPLATES_DIR"),
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		log.Printf("Ensured directory exists: %s", dir)
	}

	return nil
}

// initDatabase initializes the database connection.
func initDatabase() (*gorm.DB, error) {
	dsn := os.Getenv("STORE_DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("STORE_DATABASE_URL not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// registerHandlers registers all fulfillment handlers.
func registerHandlers(registry *handler.Registry) error {
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
