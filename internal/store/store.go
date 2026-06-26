// Пакет store реализует хранилище данных в памяти.
// Используйте отдельные мьютексы для независимых групп данных.
// Реализуйте этот пакет самостоятельно.
package store

import (
	"gopherledger/internal/domain"
	"sort"
	"sync"
	"time"
)

// Store хранит все данные приложения в памяти.
// Добавьте средства защиты конкурентного доступа самостоятельно.
type Store struct {
	usersMu      sync.RWMutex
	users        map[int64]*domain.User
	usersByLogin map[string]*domain.User

	ordersMu sync.RWMutex
	orders   map[string]*domain.Order

	balanceMu sync.RWMutex
	balances  map[int64]*domain.Balance

	withdrawalsMu sync.RWMutex
	withdrawals   map[int64][]*domain.Withdrawal

	nextIDMu sync.Mutex
	nextID   int64
}

// New создаёт и возвращает новое пустое хранилище.
func New() *Store {
	return &Store{
		users:        make(map[int64]*domain.User),
		usersByLogin: make(map[string]*domain.User),
		orders:       make(map[string]*domain.Order),
		balances:     make(map[int64]*domain.Balance),
		withdrawals:  make(map[int64][]*domain.Withdrawal),
		nextID:       1,
	}
}

// CreateUser добавляет нового пользователя.
// Возвращает domain.ErrUserExists если логин уже занят.
func (s *Store) CreateUser(login, passwordHash string) (*domain.User, error) {
	s.usersMu.Lock()
	s.nextIDMu.Lock()

	defer s.nextIDMu.Unlock()
	defer s.usersMu.Unlock()

	if _, ok := s.usersByLogin[login]; ok {
		return nil, domain.ErrUserExists
	}

	newUser := &domain.User{
		ID:           s.nextID,
		Login:        login,
		PasswordHash: passwordHash,
	}

	s.users[s.nextID] = newUser
	s.usersByLogin[login] = newUser
	s.nextID++

	return newUser, nil
}

// GetUserByLogin возвращает пользователя по логину.
// Возвращает domain.ErrUserNotFound если пользователь не найден.
func (s *Store) GetUserByLogin(login string) (*domain.User, error) {
	s.usersMu.RLock()
	defer s.usersMu.RUnlock()

	user, ok := s.usersByLogin[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

// CreateOrder добавляет новый заказ для пользователя.
// Возвращает domain.ErrOrderOwnedByUser если этот пользователь уже загружал этот номер.
// Возвращает domain.ErrOrderExists если номер принадлежит другому пользователю.
func (s *Store) CreateOrder(userID int64, number string) (*domain.Order, error) {
	s.ordersMu.Lock()
	s.nextIDMu.Lock()

	defer s.nextIDMu.Unlock()
	defer s.ordersMu.Unlock()

	if order, ok := s.orders[number]; ok {
		if order.UserID == userID {
			return nil, domain.ErrOrderOwnedByUser
		}
		return nil, domain.ErrOrderExists
	}

	order := &domain.Order{
		ID:         s.nextID,
		UserID:     userID,
		Number:     number,
		Status:     domain.OrderStatusNew,
		Accrual:    0,
		UploadedAt: time.Now(),
	}

	s.orders[number] = order
	s.nextID++

	return order, nil
}

// GetUserOrders возвращает все заказы пользователя, сначала новые.
func (s *Store) GetUserOrders(userID int64) ([]domain.Order, error) {
	s.ordersMu.RLock()
	defer s.ordersMu.RUnlock()

	userOrders := make([]domain.Order, 0)

	for _, order := range s.orders {
		if order.UserID == userID {
			userOrders = append(userOrders, *order)
		}
	}

	sort.Slice(userOrders, func(i, j int) bool {
		return userOrders[i].UploadedAt.After(userOrders[j].UploadedAt)
	})

	return userOrders, nil
}

// GetOrdersForProcessing возвращает все заказы в статусе NEW или PROCESSING.
func (s *Store) GetOrdersForProcessing() ([]domain.Order, error) {
	s.ordersMu.RLock()
	defer s.ordersMu.RUnlock()

	var orders []domain.Order

	for _, order := range s.orders {
		if order.Status == domain.OrderStatusNew ||
			order.Status == domain.OrderStatusProcessing {
			orders = append(orders, *order)
		}
	}

	return orders, nil
}

// UpdateOrderStatus обновляет статус и начисление заказа.
// Если статус PROCESSED и accrual > 0, баланс пользователя пополняется.
func (s *Store) UpdateOrderStatus(number, status string, accrual float64) error {
	s.balanceMu.Lock()
	s.ordersMu.Lock()

	defer s.ordersMu.Unlock()
	defer s.balanceMu.Unlock()

	order, ok := s.orders[number]
	if !ok {
		return domain.ErrOrderNotFound
	}

	order.Status = status
	order.Accrual = accrual

	if order.Status == domain.OrderStatusProcessed && accrual > 0 {
		balance, ok := s.balances[order.UserID]
		if !ok {
			balance = &domain.Balance{}
			s.balances[order.UserID] = balance
		}
		balance.Current += accrual
	}

	return nil
}

// GetBalance возвращает баланс пользователя.
func (s *Store) GetBalance(userID int64) (domain.Balance, error) {
	s.balanceMu.RLock()
	defer s.balanceMu.RUnlock()

	balance, ok := s.balances[userID]
	if !ok {
		return domain.Balance{}, nil
	}

	return *balance, nil
}

// Withdraw списывает сумму с баланса и записывает операцию.
// Возвращает domain.ErrInsufficientFunds если баланса не хватает.
// Обе операции должны быть атомарны: либо обе успешны, либо ни одна.
func (s *Store) Withdraw(userID int64, orderNumber string, sum float64) error {
	s.balanceMu.Lock()
	s.withdrawalsMu.Lock()
	s.nextIDMu.Lock()

	defer s.nextIDMu.Unlock()
	defer s.withdrawalsMu.Unlock()
	defer s.balanceMu.Unlock()

	balance, ok := s.balances[userID]
	if !ok || balance.Current < sum {
		return domain.ErrInsufficientFunds
	}

	balance.Current -= sum
	balance.Withdrawn -= sum

	withdrawal := &domain.Withdrawal{
		ID:          s.nextID,
		UserID:      userID,
		OrderNumber: orderNumber,
		Sum:         sum,
		ProcessedAt: time.Now(),
	}
	s.nextID++

	s.withdrawals[userID] = append(s.withdrawals[userID], withdrawal)

	return nil
}

// GetWithdrawals возвращает историю списаний пользователя, сначала новые.
func (s *Store) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	s.withdrawalsMu.RLock()
	defer s.withdrawalsMu.RUnlock()

	withdrawals := make([]domain.Withdrawal, 0)

	for _, withdrawal := range s.withdrawals[userID] {
		withdrawals = append(withdrawals, *withdrawal)
	}

	sort.Slice(withdrawals, func(i, j int) bool {
		return withdrawals[i].ProcessedAt.After(withdrawals[j].ProcessedAt)
	})

	return withdrawals, nil
}

func (s *Store) GetUsersCount() int64 {
	s.usersMu.RLock()
	defer s.usersMu.RUnlock()
	return int64(len(s.users))
}

func (s *Store) GetAllOrders() ([]domain.Order, error) {
	s.ordersMu.RLock()
	defer s.ordersMu.RUnlock()

	orders := make([]domain.Order, 0, len(s.orders))
	for _, o := range s.orders {
		orders = append(orders, *o)
	}
	return orders, nil
}

func (s *Store) GetTotalWithdrawn() float64 {
	s.balanceMu.RLock()
	defer s.balanceMu.RUnlock()

	var total float64
	for _, b := range s.balances {
		total += b.Withdrawn
	}
	return total
}
