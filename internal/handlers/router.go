package handlers

import (
	"github.com/go-chi/chi/v5"
)

func GophermartRouter(h Handlers) chi.Router {

	router := chi.NewRouter()
	router.Use(
		h.Recoverer,
		GzipHandler,
		h.CustomAuth("/api/user/register", "/api/user/login"))
	// регистрация пользователя
	router.Post("/api/user/register", h.UserRegister)
	// аутентификация пользователя
	router.Post("/api/user/login", h.UserLogin)
	// загрузка пользователем номера заказа для расчёта
	router.Post("/api/user/orders", h.OrderLoad)
	// получение списка загруженных пользователем номеров заказов, статусов их обработки и информации о начислениях
	router.Get("/api/user/orders", h.OrderGetList)
	// получение текущего баланса счёта баллов лояльности пользователя
	router.Get("/api/user/balance", h.GetBalance)
	// запрос на списание баллов с накопительного счёта в счёт оплаты нового заказа
	router.Post("/api/user/balance/withdraw", h.Withdraw)
	// получение информации о выводе средств с накопительного счёта пользователем
	router.Get("/api/user/withdrawals", h.WithdrawGetList)

	// Test
	router.Get("/api/user/checklogged", h.UserCheckLoggedInHandler)

	return router
}
