package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalhostOnly_Allows127(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := LocalhostOnly(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for localhost, got %d", w.Code)
	}
}

func TestLocalhostOnly_AllowsIPv6Loopback(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := LocalhostOnly(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for ::1, got %d", w.Code)
	}
}

func TestLocalhostOnly_BlocksExternal(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := LocalhostOnly(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for external IP, got %d", w.Code)
	}
}

func TestRequestLogger(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequestLogger_SkipsStatic(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/static/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
