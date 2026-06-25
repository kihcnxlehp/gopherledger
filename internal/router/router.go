// Пакет router собирает маршруты и middleware в единый HTTP-обработчик.
// Реализуйте этот пакет самостоятельно.
package router

import (
	"gopherledger/internal/middleware"
	"net/http"

	"gopherledger/internal/handler"
)

// New создаёт и возвращает HTTP-обработчик со всеми маршрутами.
//
// Публичные маршруты (без авторизации):
//
//	POST /api/user/register
//	POST /api/user/login
//
// Защищённые маршруты (требуют токен):
//
//	POST /api/user/orders
//	GET  /api/user/orders
//	GET  /api/user/balance
//	POST /api/user/balance/withdraw
//	GET  /api/user/withdrawals
//	POST /api/stats/export
func New(h *handler.Handler, logLevel string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/user/register", h.Register)
	mux.HandleFunc("POST /api/user/login", h.Login)

	mux.Handle("POST /api/user/orders", middleware.Auth(http.HandlerFunc(h.CreateOrder)))
	mux.Handle("GET /api/user/orders", middleware.Auth(http.HandlerFunc(h.GetOrders)))
	mux.Handle("GET /api/user/balance", middleware.Auth(http.HandlerFunc(h.GetBalance)))
	mux.Handle("POST /api/user/balance/withdraw", middleware.Auth(http.HandlerFunc(h.Withdraw)))
	mux.Handle("POST /api/user/withdrawals", middleware.Auth(http.HandlerFunc(h.GetWithdrawals)))
	mux.Handle("POST /api/stats/export", middleware.Auth(http.HandlerFunc(h.ExportStats)))

	return middleware.Recover(middleware.Logging(mux, logLevel))
}
