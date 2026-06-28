package service_test

import (
	"errors"
	"testing"
	"time"

	"gopherledger/internal/domain"
	"gopherledger/internal/service"
)

// ---------------------------------------------------------------------------
// Mock Repository
// ---------------------------------------------------------------------------

type mockRepo struct {
	users       map[string]*domain.User
	orders      map[string]*domain.Order
	balances    map[int64]*domain.Balance
	withdrawals map[int64][]*domain.Withdrawal
	nextID      int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:       make(map[string]*domain.User),
		orders:      make(map[string]*domain.Order),
		balances:    make(map[int64]*domain.Balance),
		withdrawals: make(map[int64][]*domain.Withdrawal),
		nextID:      1,
	}
}

func (m *mockRepo) CreateUser(login, passwordHash string) (*domain.User, error) {
	if _, exists := m.users[login]; exists {
		return nil, domain.ErrUserExists
	}
	user := &domain.User{ID: m.nextID, Login: login, PasswordHash: passwordHash}
	m.users[login] = user
	m.nextID++
	return user, nil
}

func (m *mockRepo) GetUserByLogin(login string) (*domain.User, error) {
	user, ok := m.users[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

func (m *mockRepo) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if order, exists := m.orders[number]; exists {
		if order.UserID == userID {
			return nil, domain.ErrOrderOwnedByUser
		}
		return nil, domain.ErrOrderExists
	}
	order := &domain.Order{
		ID: m.nextID, UserID: userID, Number: number,
		Status: domain.OrderStatusNew, UploadedAt: time.Now(),
	}
	m.orders[number] = order
	m.nextID++
	return order, nil
}

func (m *mockRepo) GetUserOrders(userID int64) ([]domain.Order, error) {
	var result []domain.Order
	for _, o := range m.orders {
		if o.UserID == userID {
			result = append(result, *o)
		}
	}
	return result, nil
}

func (m *mockRepo) GetOrdersForProcessing() ([]domain.Order, error) {
	var result []domain.Order
	for _, o := range m.orders {
		if o.Status == domain.OrderStatusNew || o.Status == domain.OrderStatusProcessing {
			result = append(result, *o)
		}
	}
	return result, nil
}

func (m *mockRepo) UpdateOrderStatus(number, status string, accrual float64) error {
	order, ok := m.orders[number]
	if !ok {
		return domain.ErrOrderNotFound
	}
	order.Status = status
	order.Accrual = accrual
	if status == domain.OrderStatusProcessed && accrual > 0 {
		b, ok := m.balances[order.UserID]
		if !ok {
			b = &domain.Balance{}
			m.balances[order.UserID] = b
		}
		b.Current += accrual
	}
	return nil
}

func (m *mockRepo) GetBalance(userID int64) (domain.Balance, error) {
	b, ok := m.balances[userID]
	if !ok {
		return domain.Balance{}, nil
	}
	return *b, nil
}

func (m *mockRepo) Withdraw(userID int64, orderNumber string, sum float64) error {
	b, ok := m.balances[userID]
	if !ok || b.Current < sum {
		return domain.ErrInsufficientFunds
	}
	b.Current -= sum
	b.Withdrawn += sum
	m.withdrawals[userID] = append(m.withdrawals[userID], &domain.Withdrawal{
		ID: m.nextID, UserID: userID, OrderNumber: orderNumber,
		Sum: sum, ProcessedAt: time.Now(),
	})
	m.nextID++
	return nil
}

func (m *mockRepo) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	var result []domain.Withdrawal
	for _, w := range m.withdrawals[userID] {
		result = append(result, *w)
	}
	return result, nil
}

func (m *mockRepo) GetUsersCount() int64 { return int64(len(m.users)) }
func (m *mockRepo) GetAllOrders() ([]domain.Order, error) {
	var result []domain.Order
	for _, o := range m.orders {
		result = append(result, *o)
	}
	return result, nil
}
func (m *mockRepo) GetTotalWithdrawn() float64 {
	var total float64
	for _, b := range m.balances {
		total += b.Withdrawn
	}
	return total
}

// ---------------------------------------------------------------------------
// Table-driven tests для RegisterUser
// ---------------------------------------------------------------------------

func TestRegisterUser(t *testing.T) {
	tests := []struct {
		name      string
		login     string
		password  string
		wantErr   error
		wantToken bool
	}{
		{
			name:      "valid registration",
			login:     "alice",
			password:  "secret123",
			wantErr:   nil,
			wantToken: true,
		},
		{
			name:      "duplicate login",
			login:     "bob",
			password:  "pass",
			wantErr:   domain.ErrUserExists,
			wantToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			// Для теста дубликата сначала регистрируем bob
			if tt.name == "duplicate login" {
				repo.CreateUser("bob", "hash")
			}

			svc := service.New(repo, time.Second, 5)

			token, err := svc.RegisterUser(tt.login, tt.password)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantToken && token == "" {
				t.Error("expected non-empty token")
			}
			if !tt.wantToken && token != "" {
				t.Errorf("expected empty token, got %s", token)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Тесты CreateOrder с валидацией Луна
// ---------------------------------------------------------------------------

func TestCreateOrder_InvalidLuhn(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo, time.Second, 5)

	_, err := svc.CreateOrder(1, "12345") // невалидный номер
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Errorf("expected ErrInvalidOrder, got: %v", err)
	}
}

func TestCreateOrder_ValidLuhn(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo, time.Second, 5)

	// Номер, проходящий проверку Луна
	order, err := svc.CreateOrder(1, "4111111111111111")
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if order.Number != "4111111111111111" {
		t.Errorf("unexpected order number")
	}
}

// ---------------------------------------------------------------------------
// Тесты GetSystemStats
// ---------------------------------------------------------------------------

func TestGetSystemStats(t *testing.T) {
	repo := newMockRepo()
	svc := service.New(repo, time.Second, 5)

	// Регистрируем пользователя и создаём заказы
	token, _ := svc.RegisterUser("alice", "pass")
	_ = token // токен не нужен для теста

	// Создаём заказы через мок напрямую
	repo.CreateOrder(1, "111111111111")
	repo.CreateOrder(1, "222222222222")
	repo.UpdateOrderStatus("111111111111", domain.OrderStatusProcessed, 100)
	repo.UpdateOrderStatus("222222222222", domain.OrderStatusInvalid, 0)

	stats, err := svc.GetSystemStats()
	if err != nil {
		t.Fatalf("GetSystemStats failed: %v", err)
	}

	if stats.UsersTotal != 1 {
		t.Errorf("expected 1 user, got %d", stats.UsersTotal)
	}
	if stats.OrdersTotal != 2 {
		t.Errorf("expected 2 orders, got %d", stats.OrdersTotal)
	}
	if stats.OrdersProcessed != 1 {
		t.Errorf("expected 1 processed, got %d", stats.OrdersProcessed)
	}
	if stats.OrdersInvalid != 1 {
		t.Errorf("expected 1 invalid, got %d", stats.OrdersInvalid)
	}
	if stats.AccrualTotal != 100 {
		t.Errorf("expected accrual 100, got %f", stats.AccrualTotal)
	}
}
