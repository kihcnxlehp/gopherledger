// Пакет service содержит бизнес-логику приложения.
//
// Взаимодействие с хранилищем осуществляется через интерфейс.
// Определите этот интерфейс здесь, по месту использования.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"math/rand"
	"sync"
	"time"

	"gopherledger/internal/auth"
	"gopherledger/internal/domain"

	"golang.org/x/sync/errgroup"
)

// Service реализует бизнес-логику приложения.
// Замените поле repo в структуре на свой интерфейс.

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

// processingOrders хранит номера заказов, которые сейчас обрабатываются воркером.
// Защитите конкурентный доступ к этому полю самостоятельно.
type Service struct {
	repo Repository

	processingMu     sync.RWMutex
	processingOrders map[string]bool

	accrualInterval   time.Duration
	workerConcurrency int
}

// New создаёт Service.
func New(repo Repository, interval time.Duration, concurrency int) *Service {
	return &Service{
		repo:              repo,
		processingOrders:  make(map[string]bool),
		accrualInterval:   interval,
		workerConcurrency: concurrency,
	}
}

// ---------------------------------------------------------------------------
// Методы бизнес-логики - реализуйте самостоятельно
// ---------------------------------------------------------------------------

// RegisterUser регистрирует нового пользователя и возвращает токен аутентификации.
// Хешируйте пароль перед сохранением с помощью crypto/sha256.
func (s *Service) RegisterUser(login, password string) (string, error) {
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

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

	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	if passwordHash != user.PasswordHash {
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
// Вызывается при загрузке заказа и при списании баллов.
func validateLuhn(number string) bool {
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

// ---------------------------------------------------------------------------
// Воркер начислений
//
// StartAccrualWorker предоставлен. Реализуйте processAllPendingOrders
// и processOrder самостоятельно.
//
// Это самая интересная часть проекта: конкурентная обработка заказов.
// Подумайте, как защитить доступ к processingOrders из нескольких горутин.
// ---------------------------------------------------------------------------

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

// processAllPendingOrders получает заказы для обработки и запускает горутины.
// Реализуйте самостоятельно.
func (s *Service) processAllPendingOrders(ctx context.Context) {
	orders, err := s.repo.GetOrdersForProcessing()
	if err != nil {
		log.Printf("воркер: ошибка получения заказов: %v", err)
		return
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.workerConcurrency)

	for _, order := range orders {
		if s.isProcessing(order.Number) {
			continue
		}
		s.markProcessing(order.Number)

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

// processOrder обрабатывает один заказ. Реализуйте самостоятельно.
// Используйте вспомогательные функции ниже для генерации случайных значений.
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

// ---------------------------------------------------------------------------
// Вспомогательные функции - предоставлены
// ---------------------------------------------------------------------------

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
