package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopherledger/internal/domain"
	"gopherledger/internal/handler"
)

// ---------------------------------------------------------------------------
// Mock Service
// ---------------------------------------------------------------------------

type mockService struct {
	registerErr error
	loginErr    error
	token       string
	orderErr    error
	balance     domain.Balance
	balanceErr  error
	withdrawErr error
	withdrawals []domain.Withdrawal
	orders      []domain.Order
	stats       *domain.SystemStats
	statsErr    error
}

func (m *mockService) RegisterUser(login, password string) (string, error) {
	if m.registerErr != nil {
		return "", m.registerErr
	}
	return m.token, nil
}

func (m *mockService) LoginUser(login, password string) (string, error) {
	if m.loginErr != nil {
		return "", m.loginErr
	}
	return m.token, nil
}

func (m *mockService) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if m.orderErr != nil {
		return nil, m.orderErr
	}
	return &domain.Order{Number: number, Status: domain.OrderStatusNew}, nil
}

func (m *mockService) GetUserOrders(userID int64) ([]domain.Order, error) {
	return m.orders, nil
}

func (m *mockService) GetBalance(userID int64) (domain.Balance, error) {
	return m.balance, m.balanceErr
}

func (m *mockService) Withdraw(userID int64, orderNumber string, sum float64) error {
	return m.withdrawErr
}

func (m *mockService) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return m.withdrawals, nil
}

func (m *mockService) GetSystemStats() (*domain.SystemStats, error) {
	return m.stats, m.statsErr
}

// ---------------------------------------------------------------------------
// Тесты Register
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	svc := &mockService{token: "test-token-123"}
	h := handler.New(svc)

	body := `{"login":"alice","password":"secret"}`
	req := httptest.NewRequest("POST", "/api/user/register", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Authorization"); got != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got '%s'", got)
	}
}

func TestRegister_EmptyBody(t *testing.T) {
	svc := &mockService{}
	h := handler.New(svc)

	req := httptest.NewRequest("POST", "/api/user/register", strings.NewReader(""))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_DuplicateUser(t *testing.T) {
	svc := &mockService{registerErr: domain.ErrUserExists}
	h := handler.New(svc)

	body := `{"login":"alice","password":"secret"}`
	req := httptest.NewRequest("POST", "/api/user/register", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Тесты CreateOrder
// ---------------------------------------------------------------------------

func TestCreateOrder_Accepted(t *testing.T) {
	svc := &mockService{}
	h := handler.New(svc)

	req := httptest.NewRequest("POST", "/api/user/orders", strings.NewReader("123456789012"))
	// Добавляем userID в контекст (имитация middleware.Auth)
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.CreateOrder(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Тесты GetBalance
// ---------------------------------------------------------------------------

func TestGetBalance_Success(t *testing.T) {
	svc := &mockService{
		balance: domain.Balance{Current: 150.5, Withdrawn: 50},
	}
	h := handler.New(svc)

	req := httptest.NewRequest("GET", "/api/user/balance", nil)
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.GetBalance(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]float64
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["current"] != 150.5 {
		t.Errorf("expected current 150.5, got %f", resp["current"])
	}
	if resp["withdrawn"] != 50 {
		t.Errorf("expected withdrawn 50, got %f", resp["withdrawn"])
	}
}

func TestGetBalance_Unauthorized(t *testing.T) {
	svc := &mockService{}
	h := handler.New(svc)

	req := httptest.NewRequest("GET", "/api/user/balance", nil)
	// НЕ добавляем userID в контекст
	w := httptest.NewRecorder()

	h.GetBalance(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Тесты Withdraw
// ---------------------------------------------------------------------------

func TestWithdraw_InsufficientFunds(t *testing.T) {
	svc := &mockService{withdrawErr: domain.ErrInsufficientFunds}
	h := handler.New(svc)

	body := `{"order":"123456789012","sum":1000}`
	req := httptest.NewRequest("POST", "/api/user/balance/withdraw", strings.NewReader(body))
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", w.Code)
	}
}

func TestWithdraw_InvalidLuhn(t *testing.T) {
	svc := &mockService{withdrawErr: domain.ErrInvalidOrder}
	h := handler.New(svc)

	body := `{"order":"12345","sum":100}`
	req := httptest.NewRequest("POST", "/api/user/balance/withdraw", strings.NewReader(body))
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.Withdraw(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Тесты GetOrders (204 No Content)
// ---------------------------------------------------------------------------

func TestGetOrders_NoContent(t *testing.T) {
	svc := &mockService{orders: []domain.Order{}}
	h := handler.New(svc)

	req := httptest.NewRequest("GET", "/api/user/orders", nil)
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.GetOrders(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestGetOrders_WithData(t *testing.T) {
	svc := &mockService{
		orders: []domain.Order{
			{Number: "111", Status: "NEW"},
			{Number: "222", Status: "PROCESSED", Accrual: 100},
		},
	}
	h := handler.New(svc)

	req := httptest.NewRequest("GET", "/api/user/orders", nil)
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.GetOrders(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp) != 2 {
		t.Errorf("expected 2 orders, got %d", len(resp))
	}
}

// ---------------------------------------------------------------------------
// Тесты ExportStats
// ---------------------------------------------------------------------------

func TestExportStats_Success(t *testing.T) {
	svc := &mockService{
		stats: &domain.SystemStats{
			UsersTotal:   10,
			OrdersTotal:  50,
			AccrualTotal: 1234.56,
		},
	}
	h := handler.New(svc)

	req := httptest.NewRequest("POST", "/api/stats/export", nil)
	ctx := handler.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	h.ExportStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
