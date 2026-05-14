package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

// CORSMiddleware adds CORS headers.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs HTTP requests.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// requireAdminToken validates admin authentication token.
func requireAdminToken(r *http.Request) error {
	token := r.Header.Get("X-Admin-Token")
	expectedToken := os.Getenv("STORE_ADMIN_TOKEN")
	if token != expectedToken || expectedToken == "" {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

// HealthHandler responds with server health status.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// rateLimiter manages per-IP rate limiting using token bucket algorithm.
type rateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	burst    int
}

// newRateLimiter creates a new rate limiter with the specified requests per minute and burst size.
func newRateLimiter(requestsPerMinute, burst int) *rateLimiter {
	return &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    rate.Limit(float64(requestsPerMinute) / 60.0), // Convert per-minute to per-second
		burst:    burst,
	}
}

// getLimiter returns the rate limiter for a given IP address.
func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.limit, rl.burst)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// RateLimitMiddleware creates a middleware that limits requests per IP address.
// requestsPerMinute: number of requests allowed per minute
// burst: burst size for token bucket (allows short bursts)
func RateLimitMiddleware(requestsPerMinute, burst int) func(http.Handler) http.Handler {
	// Check if rate limiting is enabled
	enabled := os.Getenv("STORE_RATE_LIMIT_ENABLED")
	if enabled == "false" {
		// Return a no-op middleware if disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	rl := newRateLimiter(requestsPerMinute, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			limiter := rl.getLimiter(ip)

			if !limiter.Allow() {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "rate limit exceeded",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies/load balancers)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP in the list
		return forwarded
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// GetRateLimitConfig returns rate limit configuration from environment variables.
func GetRateLimitConfig() (requestsPerMinute, burst int) {
	requestsPerMinute = 5 // Default: 5 requests per minute
	burst = 5             // Default: allow burst of 5

	if envLimit := os.Getenv("STORE_RATE_LIMIT_REQUESTS_PER_MIN"); envLimit != "" {
		if val, err := strconv.Atoi(envLimit); err == nil && val > 0 {
			requestsPerMinute = val
		}
	}

	if envBurst := os.Getenv("STORE_RATE_LIMIT_BURST"); envBurst != "" {
		if val, err := strconv.Atoi(envBurst); err == nil && val > 0 {
			burst = val
		}
	}

	return requestsPerMinute, burst
}
