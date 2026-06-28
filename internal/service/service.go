// Пакет service содержит бизнес-логику приложения.
package service

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"gopherledger/internal/auth"
	"gopherledger/internal/domain"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"
)

type Repository interface {
	CreateUser(login, passwordHash string) (*domain.User, error)
	GetUserByLogin(login string) (*domain.User, error)

	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)
	GetOrdersForProcessing() ([]domain.Order, error)
	UpdateOrderStatus(number, status string, accrual float64) error

	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)

	GetUsersCount() int64
	GetAllOrders() ([]domain.Order, error)
	GetTotalWithdrawn() float64
}

type Service struct {
	repo Repository

	processingMu     sync.RWMutex
	processingOrders map[string]bool

	accrualInterval   time.Duration
	workerConcurrency int
}

func New(repo Repository, interval time.Duration, concurrency int) *Service {
	return &Service{
		repo:              repo,
		processingOrders:  make(map[string]bool),
		accrualInterval:   interval,
		workerConcurrency: concurrency,
	}
}

// Методы бизнес-логики

// RegisterUser регистрирует нового пользователя и возвращает токен аутентификации.
func (s *Service) RegisterUser(login, password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	passwordHash := string(hash)

	user, err := s.repo.CreateUser(login, passwordHash)
	if err != nil {
		return "", err
	}

	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		return "", err
	}
	return token, nil
}

// LoginUser проверяет учётные данные и возвращает токен аутентификации.
func (s *Service) LoginUser(login, password string) (string, error) {
	user, err := s.repo.GetUserByLogin(login)
	if err != nil {
		return "", err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", domain.ErrInvalidPassword
	}

	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		return "", err
	}
	return token, nil
}

// CreateOrder проверяет номер заказа по алгоритму Луна и сохраняет заказ.
func (s *Service) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if !validateLuhn(number) {
		return nil, domain.ErrInvalidOrder
	}
	order, err := s.repo.CreateOrder(userID, number)
	if err != nil {
		return nil, err
	}
	return order, nil
}

// GetUserOrders возвращает все заказы пользователя.
func (s *Service) GetUserOrders(userID int64) ([]domain.Order, error) {
	return s.repo.GetUserOrders(userID)
}

// GetBalance возвращает текущий баланс пользователя.
func (s *Service) GetBalance(userID int64) (domain.Balance, error) {
	return s.repo.GetBalance(userID)
}

// Withdraw проверяет номер заказа по алгоритму Луна и списывает сумму с баланса.
func (s *Service) Withdraw(userID int64, orderNumber string, sum float64) error {
	if !validateLuhn(orderNumber) {
		return domain.ErrInvalidOrder
	}
	err := s.repo.Withdraw(userID, orderNumber, sum)
	if err != nil {
		return err
	}

	return nil
}

// GetWithdrawals возвращает историю списаний пользователя.
func (s *Service) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return s.repo.GetWithdrawals(userID)
}

// validateLuhn проверяет контрольную сумму номера заказа по алгоритму Луна.
func validateLuhn(number string) bool {
	if len(number) == 0 {
		return false
	}

	var sum int
	var double bool

	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}

		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		double = !double
	}

	return sum%10 == 0
}

// GetSystemStats собирает статистику из репозитория
func (s *Service) GetSystemStats() (*domain.SystemStats, error) {
	orders, err := s.repo.GetAllOrders()
	if err != nil {
		return nil, err
	}

	stats := &domain.SystemStats{
		OrdersTotal: len(orders),
	}

	for _, o := range orders {
		switch o.Status {
		case domain.OrderStatusNew:
			stats.OrdersNew++
		case domain.OrderStatusProcessing:
			stats.OrdersProcessing++
		case domain.OrderStatusProcessed:
			stats.OrdersProcessed++
			stats.AccrualTotal += o.Accrual
		case domain.OrderStatusInvalid:
			stats.OrdersInvalid++
		}
	}

	stats.WithdrawnTotal = s.repo.GetTotalWithdrawn()
	stats.UsersTotal = s.repo.GetUsersCount()

	return stats, nil
}

// StartAccrualWorker запускает фоновый цикл, который каждые 3 секунды
// передаёт необработанные заказы в processAllPendingOrders.
// Останавливается при отмене ctx.
func (s *Service) StartAccrualWorker(ctx context.Context) {
	ticker := time.NewTicker(s.accrualInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processAllPendingOrders(ctx)
		}
	}
}

// tryMarkProcessing пытается пометить заказ как обрабатываемый.
// Возвращает true, если удалось (заказ не был в обработке).
// Возвращает false, если заказ уже обрабатывается.
func (s *Service) tryMarkProcessing(orderNumber string) bool {
	s.processingMu.Lock()
	defer s.processingMu.Unlock()

	// Проверяем, не обрабатывается ли уже
	if s.processingOrders[orderNumber] {
		return false
	}

	// Помечаем как обрабатываемый
	s.processingOrders[orderNumber] = true
	return true
}

// processAllPendingOrders получает заказы для обработки и запускает горутины.
func (s *Service) processAllPendingOrders(ctx context.Context) {
	orders, err := s.repo.GetOrdersForProcessing()
	if err != nil {
		log.Printf("воркер: ошибка получения заказов: %v", err)
		return
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.workerConcurrency)

	for _, order := range orders {
		if !s.tryMarkProcessing(order.Number) {
			continue
		}

		orderNum := order.Number
		g.Go(func() error {
			defer s.unmarkProcessing(orderNum)
			s.processOrder(gctx, orderNum)
			return nil
		})
	}

	if err = g.Wait(); err != nil {
		log.Printf("воркер: ошибка группы: %v", err)
	}
}

// processOrder обрабатывает один заказ.
func (s *Service) processOrder(ctx context.Context, number string) {
	if err := s.repo.UpdateOrderStatus(number, domain.OrderStatusProcessing, 0); err != nil {
		log.Printf("воркер: ошибка обновления заказа %s: %v", number, err)
	}

	select {
	case <-ctx.Done():
		log.Printf("воркер: обработка заказа %s прервана", number)
		return
	case <-time.After(randomDelay()):

	}

	var status string
	var accrual float64

	if isInvalid() {
		status = domain.OrderStatusInvalid
		accrual = 0
	} else {
		status = domain.OrderStatusProcessed
		accrual = randomAccrual()
	}

	if err := s.repo.UpdateOrderStatus(number, status, accrual); err != nil {
		log.Printf("воркер: ошибка обновления заказа %s: %v", number, err)
	}
	log.Printf("воркер: заказ %s обработан: статус=%s, начисление=%.2f",
		number, status, accrual)
}

// isProcessing проверяет, обрабатывается ли заказ прямо сейчас
func (s *Service) isProcessing(orderNumber string) bool {
	s.processingMu.RLock()
	defer s.processingMu.RUnlock()
	return s.processingOrders[orderNumber]
}

// markProcessing помечает заказ как "в обработке"
func (s *Service) markProcessing(orderNumber string) {
	s.processingMu.Lock()
	defer s.processingMu.Unlock()
	s.processingOrders[orderNumber] = true
}

// unmarkProcessing снимает метку после завершения обработки
func (s *Service) unmarkProcessing(orderNumber string) {
	s.processingMu.Lock()
	defer s.processingMu.Unlock()
	delete(s.processingOrders, orderNumber)
}

// Вспомогательные функции

// randomAccrual возвращает случайное начисление от 10 до 500 баллов.
func randomAccrual() float64 {
	return float64(rand.Intn(491) + 10)
}

// randomDelay возвращает случайную задержку от 2 до 6 секунд.
func randomDelay() time.Duration {
	return time.Duration(rand.Intn(5)+2) * time.Second
}

// isInvalid возвращает true примерно в 10% случаев.
func isInvalid() bool {
	return rand.Intn(10) == 0
}
