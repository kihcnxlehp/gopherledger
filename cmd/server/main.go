// Точка входа сервера.
package main

import (
	"context"
	"fmt"
	"gopherledger/internal/auth"
	"gopherledger/internal/config"
	"gopherledger/internal/handler"
	"gopherledger/internal/router"
	"gopherledger/internal/service"
	"gopherledger/internal/store"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("загрузка конфига: %w", err)
	}

	auth.SetSecretKey("my-secret-key")
	auth.SetTokenTTL(24 * time.Hour)

	repo := store.New()

	svc := service.New(repo, time.Duration(cfg.AccrualInterval)*time.Second, cfg.WorkerConcurrency)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	go svc.StartAccrualWorker(workerCtx)

	h := handler.New(svc)
	r := router.New(h, cfg.LogLevel)

	addr := cfg.ServerHost + ":" + fmt.Sprintf("%d", cfg.ServerPort)
	err = gracefulShutdown(addr, cancelWorker, r)
	if err != nil {
		return err
	}
	return nil
}

func gracefulShutdown(addr string, cancel context.CancelFunc, handler http.Handler) error {
	defer cancel()

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		log.Printf("запуск сервера на %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ошибка сервера: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	<-quit
	log.Println("получен сигнал завершения, останавливаем сервер...")

	ctx, shutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdown()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при завершении сервера: %w", err)
	}

	log.Println("сервер успешно остановлен")
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
