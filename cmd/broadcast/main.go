package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	pkg "hex_toolset/pkg"
	"hex_toolset/pkg/logger"
	"hex_toolset/pkg/managers"
)

func main() {
	// Initialize logger
	logg, err := logger.New(logger.WithName("broadcast"), logger.WithConsole(true), logger.WithJSON(true))
	if err != nil {
		fmt.Printf("failed to init logger: %v\n", err)
		return
	}
	defer logg.Close()

	cfg := pkg.GetConfig()
	mgr := managers.NewBroadcastManager(cfg, logg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logg.Warnf("shutdown signal received")
		mgr.Stop()
		cancel()
	}()

	if err := mgr.Run(ctx); err != nil {
		logg.Errorf("broadcast manager exited with error: %v", err)
	}
}
