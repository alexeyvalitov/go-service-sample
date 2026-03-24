package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexeyvalitov/go-service-sample/internal/app"
	"github.com/alexeyvalitov/go-service-sample/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Printf("config error: %v", err)
		os.Exit(2)
	}

	service, err := app.New(cfg)
	if err != nil {
		log.Printf("app init error: %v", err)
		os.Exit(2)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Run(sigCtx); err != nil {
		log.Printf("app error: %v", err)
		os.Exit(1)
	}
}
