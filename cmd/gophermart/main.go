package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/handlers"
	"yapracticum-go-diploma-1/internal/storage"
)

var logger *zap.Logger
var dbStorage *storage.Storage

func Router() chi.Router {
	router := chi.NewRouter()
	router.Use(
		middleware.Recoverer,
		handlers.GzipHandler,
		handlers.CustomAuth("/api/user/register", "/api/user/login"))
	// регистрация пользователя
	router.Post("/api/user/register", handlers.UserRegister)
	// аутентификация пользователя
	router.Post("/api/user/login", handlers.UserLogin)
	// загрузка пользователем номера заказа для расчёта
	router.Post("/api/user/orders", handlers.OrderLoad)
	// получение списка загруженных пользователем номеров заказов, статусов их обработки и информации о начислениях
	router.Get("/api/user/orders", handlers.OrderGetList)
	// получение текущего баланса счёта баллов лояльности пользователя
	router.Get("/api/user/balance", handlers.GetBalance)
	// запрос на списание баллов с накопительного счёта в счёт оплаты нового заказа
	router.Post("/api/user/balance/withdraw", handlers.Withdraw)
	// получение информации о выводе средств с накопительного счёта пользователем
	router.Get("/api/user/withdrawals", handlers.WithdrawGetList)

	// Test
	router.Get("/api/user/checklogged", handlers.UserCheckLoggedInHandler)

	return router
}

func main() {
	parentContext := context.Background()

	var err error
	if logger, err = zap.NewProduction(); err != nil {
		panic(err)
	}

	cfg := config.New()
	dbStorage = storage.New(cfg.ConnString)
	if err = dbStorage.Init(parentContext, logger, cfg); err != nil {
		logger.Error(err.Error())
		return
	}

	handlers.Init(logger, dbStorage, cfg)

	server := http.Server{Addr: cfg.Endpoint, Handler: Router()}

	go shutdownSignal(parentContext, &server)

	if err := server.ListenAndServe(); err != nil {
		logger.Error(err.Error())
	}
}

func shutdownSignal(ctx context.Context, server *http.Server) {
	terminateSignals := make(chan os.Signal, 1)
	signal.Notify(terminateSignals, syscall.SIGTERM, syscall.SIGINT)
	s := <-terminateSignals
	logger.Info("Got one of stop signals, shutting down server gracefully, SIGNAL NAME :" + s.String())
	dbStorage.Close(ctx)
	server.Shutdown(ctx)
}
