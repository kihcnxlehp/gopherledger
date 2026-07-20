// Пакет middleware содержит HTTP-middleware.
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"gopherledger/internal/auth"
	"gopherledger/internal/handler"
	"log"
	"net/http"
	"time"
)

// Auth проверяет токен из заголовка Authorization и помещает ID пользователя в контекст.
// Запросы без валидного токена получают ответ 401 Unauthorized.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
			return
		}

		id, err := auth.ValidateToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token", "недействительный токен", err)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, handler.CtxKeyUserID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder оборачивает http.ResponseWriter для перехвата статус-кода.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// Logging логирует метод, путь, статус ответа и время выполнения каждого запроса.
func Logging(next http.Handler, logLevel string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		duration := time.Since(start)
		baseLog := fmt.Sprintf("%s %s %d %v", r.Method, r.URL.Path, recorder.status, duration.Round(time.Millisecond))

		if logLevel == "debug" {
			log.Printf("[DEBUG] %s | Query: %q | User-Agent: %q | Content-Type: %q",
				baseLog, r.URL.RawQuery, r.UserAgent(), r.Header.Get("Content-Type"))
		} else {
			log.Println(baseLog)
		}
	})
}

// Recover перехватывает панику внутри handler, логирует её и возвращает
// клиенту ответ 500 Internal Server Error вместо того, чтобы уронить сервер.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Вспомогательные функции для ответов

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError записывает JSON-ответ с ошибкой.
// Клиент видит только userMsg. Внутренние детали пишутся только в лог.
func writeError(w http.ResponseWriter, status int, code, userMsg string, internalErr error) {
	if internalErr != nil {
		log.Printf("ошибка code=%s status=%d: %v", code, status, internalErr)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		Code:    code,
		Message: userMsg,
	}

	_ = json.NewEncoder(w).Encode(response)
}
