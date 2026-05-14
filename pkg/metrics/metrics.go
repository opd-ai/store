package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PaymentsTotal tracks the total number of payments by status
	PaymentsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_payments_total",
			Help: "Total number of payments by status",
		},
		[]string{"status"}, // pending, confirmed, fulfilled, failed
	)

	// FulfillmentDuration tracks how long fulfillment takes
	FulfillmentDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "store_fulfillment_duration_seconds",
			Help:    "Duration of fulfillment operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler_type"}, // digital_media, shipping_form, pod, custom
	)

	// CheckoutErrors tracks checkout failures
	CheckoutErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_checkout_errors_total",
			Help: "Total number of checkout errors by reason",
		},
		[]string{"reason"}, // invalid_item, handler_error, paywall_error, etc.
	)

	// HandlerErrors tracks handler-specific errors
	HandlerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_handler_errors_total",
			Help: "Total number of handler errors by type",
		},
		[]string{"handler_type"},
	)

	// HTTPRequests tracks HTTP request counts
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_http_requests_total",
			Help: "Total number of HTTP requests by method, path, and status",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks HTTP request latency
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "store_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// DatabaseOperations tracks database operation counts
	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_database_operations_total",
			Help: "Total number of database operations by type",
		},
		[]string{"operation"}, // put, get, delete, view, update
	)

	// ActiveCheckouts tracks number of active checkout sessions
	ActiveCheckouts = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "store_active_checkouts",
			Help: "Number of currently active checkout sessions",
		},
	)
)

// RecordPayment increments the payment counter for the given status
func RecordPayment(status string) {
	PaymentsTotal.WithLabelValues(status).Inc()
}

// RecordFulfillment records the duration of a fulfillment operation
func RecordFulfillment(handlerType string, durationSeconds float64) {
	FulfillmentDuration.WithLabelValues(handlerType).Observe(durationSeconds)
}

// RecordCheckoutError increments the checkout error counter
func RecordCheckoutError(reason string) {
	CheckoutErrors.WithLabelValues(reason).Inc()
}

// RecordHandlerError increments the handler error counter
func RecordHandlerError(handlerType string) {
	HandlerErrors.WithLabelValues(handlerType).Inc()
}

// RecordHTTPRequest increments the HTTP request counter
func RecordHTTPRequest(method, path, status string) {
	HTTPRequests.WithLabelValues(method, path, status).Inc()
}

// RecordHTTPDuration records the latency of an HTTP request
func RecordHTTPDuration(method, path string, durationSeconds float64) {
	HTTPRequestDuration.WithLabelValues(method, path).Observe(durationSeconds)
}

// RecordDatabaseOperation increments the database operation counter
func RecordDatabaseOperation(operation string) {
	DatabaseOperations.WithLabelValues(operation).Inc()
}

// IncrementActiveCheckouts increments the active checkouts gauge
func IncrementActiveCheckouts() {
	ActiveCheckouts.Inc()
}

// DecrementActiveCheckouts decrements the active checkouts gauge
func DecrementActiveCheckouts() {
	ActiveCheckouts.Dec()
}
