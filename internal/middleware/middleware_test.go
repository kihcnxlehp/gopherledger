package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gopherledger/internal/auth"
	"gopherledger/internal/handler"
	"gopherledger/internal/middleware"
)

func TestAuth_ValidToken(t *testing.T) {
	// Генерируем токен
	token, _ := auth.GenerateToken(42)

	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := handler.UserIDFromContext(r.Context())
		if !ok {
			t.Error("userID not in context")
		}
		if userID != 42 {
			t.Errorf("expected userID 42, got %d", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.Auth(handlerFunc)

	req := httptest.NewRequest("GET", "/api/user/balance", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuth_MissingToken(t *testing.T) {
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrapped := middleware.Auth(handlerFunc)

	req := httptest.NewRequest("GET", "/api/user/balance", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrapped := middleware.Auth(handlerFunc)

	req := httptest.NewRequest("GET", "/api/user/balance", nil)
	req.Header.Set("Authorization", "invalid-token")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRecover_PanicHandled(t *testing.T) {
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	wrapped := middleware.Recover(handlerFunc)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Не должно упасть
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
