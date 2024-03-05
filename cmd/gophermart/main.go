package main

import (
	"context"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"yapracticum-go-diploma-1/internal/accrualpoll"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/handlers"
	"yapracticum-go-diploma-1/internal/storage"
	"yapracticum-go-diploma-1/internal/utils"
)

var logger *zap.Logger
var dbStorage *storage.Storage

func main() {
	parentContext, cancel := context.WithCancel(context.Background())

	var err error
	if logger, err = zap.NewProduction(); err != nil {
		panic(err)
	}

	cfg := config.New()
	newOrdersCh := make(chan storage.OrderTag, 1000)
	dbStorage, err = storage.New(cfg, logger, newOrdersCh)
	if err != nil {
		panic(err.Error())
	}

	dbStorage.Init(parentContext)

	workersWg := sync.WaitGroup{}
	ccw := utils.NewCtxCancelWaiter(parentContext, 0)
	accrualPoll := accrualpoll.NewAccrualPollWorker(ccw, dbStorage, &workersWg, logger, cfg.AccrualAddress, newOrdersCh)
	accrualPoll.StartPoll(5)
	go accrualPoll.GetUnhandledOrders(parentContext)

	h := handlers.Handlers{Logger: logger, DBStorage: dbStorage, Cfg: cfg}
	server := http.Server{Addr: cfg.Endpoint, Handler: handlers.GophermartRouter(h)}

	go shutdownSignal(parentContext, cancel, &workersWg, newOrdersCh, &server)

	if err := server.ListenAndServe(); err != nil {
		logger.Error(err.Error())
	}
}

func shutdownSignal(ctx context.Context, cancel context.CancelFunc, workersWg *sync.WaitGroup, newOrdersCh chan storage.OrderTag, server *http.Server) {
	terminateSignals := make(chan os.Signal, 1)
	signal.Notify(terminateSignals, syscall.SIGTERM, syscall.SIGINT)
	s := <-terminateSignals
	logger.Info("Got one of stop signals, shutting down server gracefully, SIGNAL NAME :" + s.String())
	dbStorage.Close(ctx)
	cancel()
	workersWg.Wait()
	close(newOrdersCh)
	server.Shutdown(ctx)
}
