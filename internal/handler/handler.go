// Пакет handler содержит HTTP-обработчики.
//
// Взаимодействие с бизнес-логикой осуществляется через интерфейс.
// Определите этот интерфейс здесь, по месту использования.
// Реализуйте все обработчики самостоятельно.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gopherledger/internal/domain"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

type Service interface {
	RegisterUser(login, password string) (string, error)
	LoginUser(login, password string) (string, error)

	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)

	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)

	GetSystemStats() (*domain.SystemStats, error)
}

// Handler хранит зависимость от бизнес-логики.
// Замените interface{} на свой интерфейс.
type Handler struct {
	svc Service
}

// New создаёт Handler.
func New(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ---------------------------------------------------------------------------
// Вспомогательные функции для ответов - предоставлены
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

// writeJSON записывает успешный JSON-ответ.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Обработчики - реализуйте самостоятельно
// ---------------------------------------------------------------------------

type registerRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Register обрабатывает POST /api/user/register.
// При успехе: 200 OK, заголовок Authorization с токеном.
// При дублировании логина: 409 Conflict.
// При некорректных данных: 400 Bad Request.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req registerRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "parse_error", "некорректный формат запроса", err)
		return
	}

	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "login и password обязательны", nil)
		return
	}

	token, err := h.svc.RegisterUser(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrUserExists) {
			writeError(w, http.StatusConflict, "user_exists", "пользователь уже существует", err)
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		}
		return
	}

	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// Login обрабатывает POST /api/user/login.
// При успехе: 200 OK, заголовок Authorization с токеном.
// При неверных данных: 401 Unauthorized.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req registerRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "parse_error", "некорректный формат запроса", err)
		return
	}

	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "login и password обязательны", nil)
		return
	}

	token, err := h.svc.LoginUser(req.Login, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "неверный логин или пароль", err)
		return
	}

	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// CreateOrder обрабатывает POST /api/user/orders.
// Тело запроса: номер заказа в виде обычного текста.
// 202 Accepted  - новый заказ принят в обработку.
// 200 OK        - заказ уже загружен этим пользователем.
// 409 Conflict  - заказ принадлежит другому пользователю.
// 422 Unprocessable Entity - номер не прошёл проверку Луна.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read_error", "не удалось прочитать запрос", err)
		return
	}

	orderNumber := strings.TrimSpace(string(body))
	if orderNumber == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "номер заказа обязателен", nil)
		return
	}

	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	_, err = h.svc.CreateOrder(userID, orderNumber)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidOrder) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_order", "номер не прошел проверку Луна", err)
		} else if errors.Is(err, domain.ErrOrderOwnedByUser) {
			w.WriteHeader(http.StatusOK)
		} else if errors.Is(err, domain.ErrOrderExists) {
			writeError(w, http.StatusConflict, "order_exists", "заказ принадлежит другому пользователю", err)
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		}
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

type orderResponse struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    float64   `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// GetOrders обрабатывает GET /api/user/orders.
// 200 OK с JSON-массивом заказов или 204 No Content если заказов нет.
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	orders, err := h.svc.GetUserOrders(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]orderResponse, len(orders))
	for i, o := range orders {
		resp[i] = orderResponse{
			Number:     o.Number,
			Status:     o.Status,
			Accrual:    o.Accrual,
			UploadedAt: o.UploadedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type balanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

// GetBalance обрабатывает GET /api/user/balance.
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	balance, err := h.svc.GetBalance(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		return
	}

	resp := balanceResponse{
		Current:   balance.Current,
		Withdrawn: balance.Withdrawn,
	}

	writeJSON(w, http.StatusOK, resp)
}

type withdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

// Withdraw обрабатывает POST /api/user/balance/withdraw.
// 200 OK при успехе.
// 402 Payment Required при нехватке баллов.
// 422 Unprocessable Entity при неверном номере заказа.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	defer r.Body.Close()

	var req withdrawRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "parse_error", "некорректный формат запроса", err)
		return
	}

	if req.Order == "" || req.Sum <= 0 || math.IsNaN(req.Sum) || math.IsInf(req.Sum, 0) {
		writeError(w, http.StatusBadRequest, "validation_error", "пустой order или некорректная сумма списания", nil)
		return
	}

	err := h.svc.Withdraw(userID, req.Order, req.Sum)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidOrder) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_order", "номер не прошел проверку Луна", err)
		} else if errors.Is(err, domain.ErrInsufficientFunds) {
			writeError(w, http.StatusPaymentRequired, "payment_required", "недостаточно баллов", err)
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

type withdrawResponse struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

// GetWithdrawals обрабатывает GET /api/user/withdrawals.
// 200 OK с массивом или 204 No Content если списаний нет.
func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	withdraws, err := h.svc.GetWithdrawals(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сервера", err)
		return
	}

	if len(withdraws) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]withdrawResponse, len(withdraws))
	for i, withdraw := range withdraws {
		resp[i] = withdrawResponse{
			Order:       withdraw.OrderNumber,
			Sum:         withdraw.Sum,
			ProcessedAt: withdraw.ProcessedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ExportStats обрабатывает POST /api/stats/export.
// Собирает статистику системы и записывает её в текстовый файл stats.txt
// в корне проекта. Возвращает 200 OK при успехе.
//
// Файл должен содержать:
//   - общее число зарегистрированных пользователей
//   - общее число заказов и их распределение по статусам
//   - суммарное количество начисленных баллов
//   - суммарное количество списанных баллов
//   - время генерации отчёта
//
// Для работы с файлами используйте пакет os (неделя 8).
func (h *Handler) ExportStats(w http.ResponseWriter, r *http.Request) {
	_, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "требуется авторизация", nil)
		return
	}

	stats, err := h.svc.GetSystemStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "ошибка сбора статистики", err)
		return
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "users: %d\n", stats.UsersTotal)
	fmt.Fprintf(&buf, "orders_total: %d\n", stats.OrdersTotal)
	fmt.Fprintf(&buf, "orders_new: %d\n", stats.OrdersNew)
	fmt.Fprintf(&buf, "orders_processing: %d\n", stats.OrdersProcessing)
	fmt.Fprintf(&buf, "orders_processed: %d\n", stats.OrdersProcessed)
	fmt.Fprintf(&buf, "orders_invalid: %d\n", stats.OrdersInvalid)
	fmt.Fprintf(&buf, "accrual_total: %.2f\n", stats.AccrualTotal)
	fmt.Fprintf(&buf, "withdrawn_total: %.2f\n", stats.WithdrawnTotal)
	fmt.Fprintf(&buf, "generated_at: %s\n", time.Now().Format(time.RFC3339))

	const targetFile = "stats.txt"
	const tmpFile = targetFile + ".tmp"

	if err = os.WriteFile(tmpFile, []byte(buf.String()), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "file_error", "не удалось записать файл", err)
		return
	}

	if err = os.Rename(tmpFile, targetFile); err != nil {
		os.Remove(tmpFile)
		writeError(w, http.StatusInternalServerError, "file_error", "не удалось обновить файл статистики", err)
	}

	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Вспомогательная функция для работы с контекстом - предоставлена
// ---------------------------------------------------------------------------

type contextKey string

const CtxKeyUserID contextKey = "userID"

// UserIDFromContext извлекает ID аутентифицированного пользователя из контекста.
// Возвращает 0, false если значение отсутствует.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(CtxKeyUserID).(int64)
	return userID, ok
}
