package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"yapracticum-go-diploma-1/internal/accrlaupoll"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/handlers"
	"yapracticum-go-diploma-1/internal/storage"
	"yapracticum-go-diploma-1/internal/utils"
)

var logger *zap.Logger
var dbStorage *storage.Storage

func Router(h handlers.Handlers) chi.Router {

	router := chi.NewRouter()
	router.Use(
		h.Recoverer,
		handlers.GzipHandler,
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

func main() {
	parentContext, cancel := context.WithCancel(context.Background())

	var err error
	if logger, err = zap.NewProduction(); err != nil {
		panic(err)
	}

	cfg := config.New()
	newOrdersCh := make(chan string, 1000)
	dbStorage, err = storage.New(cfg, logger, newOrdersCh)
	if err != nil {
		panic(err.Error())
	}

	dbStorage.Init(parentContext)

	workersWg := sync.WaitGroup{}
	ccw := utils.NewCtxCancellWaiter(parentContext, 0)
	for i := 1; i <= 5; i++ {
		go accrlaupoll.AccrualPollWorker(ccw, dbStorage, i, &workersWg, logger, cfg.AccrualAddress, newOrdersCh)
	}

	h := handlers.Handlers{Logger: logger, DBStorage: dbStorage, Cfg: cfg}
	server := http.Server{Addr: cfg.Endpoint, Handler: Router(h)}

	go shutdownSignal(parentContext, cancel, &workersWg, &server)

	if err := server.ListenAndServe(); err != nil {
		logger.Error(err.Error())
	}
}

func shutdownSignal(ctx context.Context, cancel context.CancelFunc, workersWg *sync.WaitGroup, server *http.Server) {
	terminateSignals := make(chan os.Signal, 1)
	signal.Notify(terminateSignals, syscall.SIGTERM, syscall.SIGINT)
	s := <-terminateSignals
	logger.Info("Got one of stop signals, shutting down server gracefully, SIGNAL NAME :" + s.String())
	dbStorage.Close(ctx)
	cancel()
	workersWg.Wait()
	server.Shutdown(ctx)
}
