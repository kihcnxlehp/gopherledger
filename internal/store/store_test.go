package store_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gopherledger/internal/domain"
	"gopherledger/internal/store"
)

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New()
}

// ---------------------------------------------------------------------------
// Тесты CreateUser
// ---------------------------------------------------------------------------

func TestCreateUser_Success(t *testing.T) {
	s := newTestStore(t)

	user, err := s.CreateUser("testuser", "hashed_password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if user.Login != "testuser" {
		t.Errorf("expected login 'testuser', got '%s'", user.Login)
	}
	if user.PasswordHash != "hashed_password" {
		t.Errorf("unexpected password hash")
	}
}

func TestCreateUser_DuplicateLogin(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateUser("testuser", "hash1")
	if err != nil {
		t.Fatalf("first CreateUser failed: %v", err)
	}

	_, err = s.CreateUser("testuser", "hash2")
	if !errors.Is(err, domain.ErrUserExists) {
		t.Errorf("expected ErrUserExists, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Тесты GetUserByLogin
// ---------------------------------------------------------------------------

func TestGetUserByLogin_Found(t *testing.T) {
	s := newTestStore(t)

	created, _ := s.CreateUser("alice", "hash")

	found, err := s.GetUserByLogin("alice")
	if err != nil {
		t.Fatalf("GetUserByLogin failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, found.ID)
	}
}

func TestGetUserByLogin_NotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetUserByLogin("nonexistent")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Тесты CreateOrder
// ---------------------------------------------------------------------------

func TestCreateOrder_Success(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")

	order, err := s.CreateOrder(user.ID, "123456789012")
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	if order.Status != domain.OrderStatusNew {
		t.Errorf("expected status NEW, got %s", order.Status)
	}
	if order.UserID != user.ID {
		t.Errorf("expected userID %d, got %d", user.ID, order.UserID)
	}
}

func TestCreateOrder_OwnedByUser(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	s.CreateOrder(user.ID, "123456789012")

	_, err := s.CreateOrder(user.ID, "123456789012")
	if !errors.Is(err, domain.ErrOrderOwnedByUser) {
		t.Errorf("expected ErrOrderOwnedByUser, got: %v", err)
	}
}

func TestCreateOrder_OwnedByOtherUser(t *testing.T) {
	s := newTestStore(t)

	user1, _ := s.CreateUser("alice", "hash")
	user2, _ := s.CreateUser("bob", "hash")

	s.CreateOrder(user1.ID, "123456789012")

	_, err := s.CreateOrder(user2.ID, "123456789012")
	if !errors.Is(err, domain.ErrOrderExists) {
		t.Errorf("expected ErrOrderExists, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Тесты GetUserOrders (сортировка)
// ---------------------------------------------------------------------------

func TestGetUserOrders_SortedByDateDesc(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")

	// Создаём заказы с небольшой задержкой для разных timestamps
	s.CreateOrder(user.ID, "111111111111")
	time.Sleep(10 * time.Millisecond)
	s.CreateOrder(user.ID, "222222222222")
	time.Sleep(10 * time.Millisecond)
	s.CreateOrder(user.ID, "333333333333")

	orders, err := s.GetUserOrders(user.ID)
	if err != nil {
		t.Fatalf("GetUserOrders failed: %v", err)
	}

	if len(orders) != 3 {
		t.Fatalf("expected 3 orders, got %d", len(orders))
	}

	// Проверяем сортировку по убыванию времени
	for i := 0; i < len(orders)-1; i++ {
		if orders[i].UploadedAt.Before(orders[i+1].UploadedAt) {
			t.Errorf("orders not sorted by date desc: %v before %v",
				orders[i].UploadedAt, orders[i+1].UploadedAt)
		}
	}

	// Последний созданный заказ должен быть первым
	if orders[0].Number != "333333333333" {
		t.Errorf("expected first order to be 333333333333, got %s", orders[0].Number)
	}
}

func TestGetUserOrders_Empty(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")

	orders, err := s.GetUserOrders(user.ID)
	if err != nil {
		t.Fatalf("GetUserOrders failed: %v", err)
	}

	if len(orders) != 0 {
		t.Errorf("expected 0 orders, got %d", len(orders))
	}
}

// ---------------------------------------------------------------------------
// Тесты UpdateOrderStatus и баланс
// ---------------------------------------------------------------------------

func TestUpdateOrderStatus_Processed_AddsAccrual(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")

	err := s.UpdateOrderStatus(order.Number, domain.OrderStatusProcessed, 150.50)
	if err != nil {
		t.Fatalf("UpdateOrderStatus failed: %v", err)
	}

	balance, err := s.GetBalance(user.ID)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance.Current != 150.50 {
		t.Errorf("expected balance 150.50, got %f", balance.Current)
	}
}

func TestUpdateOrderStatus_Processed_ZeroAccrual_NoChange(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")

	err := s.UpdateOrderStatus(order.Number, domain.OrderStatusProcessed, 0)
	if err != nil {
		t.Fatalf("UpdateOrderStatus failed: %v", err)
	}

	balance, _ := s.GetBalance(user.ID)
	if balance.Current != 0 {
		t.Errorf("expected balance 0, got %f", balance.Current)
	}
}

func TestUpdateOrderStatus_Invalid_NoAccrual(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")

	err := s.UpdateOrderStatus(order.Number, domain.OrderStatusInvalid, 100)
	if err != nil {
		t.Fatalf("UpdateOrderStatus failed: %v", err)
	}

	balance, _ := s.GetBalance(user.ID)
	if balance.Current != 0 {
		t.Errorf("expected balance 0 for invalid order, got %f", balance.Current)
	}
}

// ---------------------------------------------------------------------------
// Тесты Withdraw
// ---------------------------------------------------------------------------

func TestWithdraw_Success(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")
	s.UpdateOrderStatus(order.Number, domain.OrderStatusProcessed, 200)

	err := s.Withdraw(user.ID, "123456789012", 50)
	if err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}

	balance, _ := s.GetBalance(user.ID)
	if balance.Current != 150 {
		t.Errorf("expected balance 150, got %f", balance.Current)
	}
	if balance.Withdrawn != 50 {
		t.Errorf("expected withdrawn 50, got %f", balance.Withdrawn)
	}
}

func TestWithdraw_InsufficientFunds(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	s.CreateOrder(user.ID, "123456789012")

	err := s.Withdraw(user.ID, "123456789012", 100)
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got: %v", err)
	}
}

func TestWithdraw_RecordsHistory(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")
	s.UpdateOrderStatus(order.Number, domain.OrderStatusProcessed, 200)

	s.Withdraw(user.ID, "123456789012", 50)
	time.Sleep(10 * time.Millisecond)
	s.Withdraw(user.ID, "123456789012", 30)

	withdrawals, _ := s.GetWithdrawals(user.ID)
	if len(withdrawals) != 2 {
		t.Fatalf("expected 2 withdrawals, got %d", len(withdrawals))
	}

	// Проверяем сортировку: последние сначала
	if withdrawals[0].Sum != 30 {
		t.Errorf("expected first withdrawal sum 30, got %f", withdrawals[0].Sum)
	}
	if withdrawals[1].Sum != 50 {
		t.Errorf("expected second withdrawal sum 50, got %f", withdrawals[1].Sum)
	}
}

// ---------------------------------------------------------------------------
// Тесты статистики
// ---------------------------------------------------------------------------

func TestGetSystemStats(t *testing.T) {
	s := newTestStore(t)

	user1, _ := s.CreateUser("alice", "hash")
	user2, _ := s.CreateUser("bob", "hash")

	order1, _ := s.CreateOrder(user1.ID, "111111111111")
	order2, _ := s.CreateOrder(user2.ID, "222222222222")
	order3, _ := s.CreateOrder(user1.ID, "333333333333")

	s.UpdateOrderStatus(order1.Number, domain.OrderStatusProcessed, 100)
	s.UpdateOrderStatus(order2.Number, domain.OrderStatusInvalid, 0)
	// order3 остаётся NEW

	s.Withdraw(user1.ID, "111111111111", 30)

	usersCount := s.GetUsersCount()
	if usersCount != 2 {
		t.Errorf("expected 2 users, got %d", usersCount)
	}

	allOrders, _ := s.GetAllOrders()
	if len(allOrders) != 3 {
		t.Errorf("expected 3 orders, got %d", len(allOrders))
	}

	withdrawn := s.GetTotalWithdrawn()
	if withdrawn != 30 {
		t.Errorf("expected withdrawn total 30, got %f", withdrawn)
	}

	if order3.Status != domain.OrderStatusNew {
		t.Errorf("expected order3 status NEW, got %s", order3.Status)
	}
}

// ---------------------------------------------------------------------------
// Тесты на race condition (запускать с -race)
// ---------------------------------------------------------------------------

func TestStore_ConcurrentAccess(t *testing.T) {
	s := newTestStore(t)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Параллельно создаём пользователей
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			login := "user_" + string(rune('A'+idx%26)) + string(rune('0'+idx/26))
			s.CreateUser(login, "hash")
		}(i)
	}

	wg.Wait()

	count := s.GetUsersCount()
	if count != int64(numGoroutines) {
		t.Errorf("expected %d users, got %d", numGoroutines, count)
	}
}

func TestStore_ConcurrentOrdersAndWithdrawals(t *testing.T) {
	s := newTestStore(t)

	user, _ := s.CreateUser("alice", "hash")
	order, _ := s.CreateOrder(user.ID, "123456789012")
	s.UpdateOrderStatus(order.Number, domain.OrderStatusProcessed, 1000)

	var wg sync.WaitGroup
	numOps := 20

	// Параллельно делаем списания
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Withdraw(user.ID, "123456789012", 1)
		}()
	}

	wg.Wait()

	balance, _ := s.GetBalance(user.ID)
	expectedCurrent := float64(1000 - numOps)
	if balance.Current != expectedCurrent {
		t.Errorf("expected balance %f, got %f", expectedCurrent, balance.Current)
	}
	if balance.Withdrawn != float64(numOps) {
		t.Errorf("expected withdrawn %f, got %f", float64(numOps), balance.Withdrawn)
	}
}
