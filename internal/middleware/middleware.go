// Пакет middleware содержит HTTP-middleware.
// Реализуйте Auth, Logging и Recover самостоятельно.
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
//
// Что нужно сделать:
//   - прочитать токен из заголовка
//   - проверить токен через пакет auth
//   - поместить ID пользователя в контекст запроса
//   - передать управление следующему handler или вернуть 401
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
// Используйте эту структуру в Logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// Logging логирует метод, путь, статус ответа и время выполнения каждого запроса.
//
// Что нужно сделать:
//   - зафиксировать время начала запроса
//   - обернуть w в statusRecorder для перехвата статус-кода
//   - после выполнения handler записать лог
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
//
// Что нужно сделать:
//   - добавить defer с вызовом recover()
//   - если паника произошла, залогировать её и отдать 500
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

// ---------------------------------------------------------------------------
// Вспомогательные функции для ответов
// ---------------------------------------------------------------------------

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError записывает JSON-ответ с ошибкой.
// Клиент видит только userMsg. Внутренние детали пишутся только в лог.
// Прочитайте ТЗ и создайте структуру тела ответа самостоятельно.
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

	json.NewEncoder(w).Encode(response)
}
