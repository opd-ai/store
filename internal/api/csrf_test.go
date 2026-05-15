package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCSRFMiddleware_GET_NoValidation(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for GET request, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_POST_ValidToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	token := "test-csrf-token"
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader("{}"))
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for POST with valid token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_POST_MissingToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/test", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for POST without token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_POST_MismatchedToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/test", strings.NewReader("{}"))
	req.Header.Set("X-CSRF-Token", "token1")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token2"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for mismatched tokens, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_Disabled(t *testing.T) {
	originalValue := os.Getenv("STORE_CSRF_ENABLED")
	defer os.Setenv("STORE_CSRF_ENABLED", originalValue)
	os.Setenv("STORE_CSRF_ENABLED", "false")

	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("POST", "/api/test", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 when CSRF is disabled, got %d", rec.Code)
	}
}

func TestGetCSRFToken(t *testing.T) {
	// Create a minimal handler
	h := &Handler{}

	req := httptest.NewRequest("GET", "/api/csrf-token", nil)
	rec := httptest.NewRecorder()

	h.GetCSRFToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Check that cookie was set
	cookies := rec.Result().Cookies()
	foundCookie := false
	for _, cookie := range cookies {
		if cookie.Name == "csrf_token" {
			foundCookie = true
			if cookie.Value == "" {
				t.Error("CSRF token cookie value is empty")
			}
			if cookie.HttpOnly != true {
				t.Error("CSRF token cookie should be HttpOnly")
			}
			if cookie.MaxAge != 3600 {
				t.Errorf("CSRF token cookie MaxAge should be 3600, got %d", cookie.MaxAge)
			}
		}
	}
	if !foundCookie {
		t.Error("CSRF token cookie was not set")
	}

	// Check response body contains token
	body := rec.Body.String()
	if !strings.Contains(body, "csrf_token") {
		t.Error("Response should contain csrf_token")
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	token1 := generateCSRFToken()
	token2 := generateCSRFToken()

	if token1 == "" {
		t.Error("generateCSRFToken returned empty string")
	}

	if token1 == token2 {
		t.Error("generateCSRFToken should generate unique tokens")
	}

	// Check token length (32 bytes base64 encoded should be ~44 characters)
	if len(token1) < 40 {
		t.Errorf("Token seems too short: %d characters", len(token1))
	}
}

func TestCSRFMiddleware_HEAD_NoValidation(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("HEAD", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for HEAD request, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_OPTIONS_NoValidation(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS request, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_PUT_RequiresToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("PUT", "/api/test", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for PUT without token, got %d", rec.Code)
	}
}

func TestCSRFMiddleware_DELETE_RequiresToken(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for DELETE without token, got %d", rec.Code)
	}
}
