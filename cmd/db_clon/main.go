package main

import (
	"context"
	"fmt"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/managers"

	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Root context that cancels on SIGINT/SIGTERM for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err := db.GetInstance().InitDefault(ctx)
	if err != nil {
		fmt.Printf("Error initializing database: %v\n", err)
		return
	}

	err = db.GetInstance().HealthCheck(ctx)

	if err != nil {
		fmt.Printf("Error checking database health: %v\n", err)
		return
	}

	fmt.Println("DB initialized")

	// Initialize managers with the long-lived context
	sfcManager := managers.NewSFCAPIManager(&ctx)
	lm := managers.NewLoopsManager(ctx)
	defer lm.Stop() // ensure loops are stopped on exit

	// Start loops (run in parallel)
	lm.StartEveryMinute(func(ctx context.Context, minute time.Time) {
		sfcManager.RequestMinute(minute)
	})

	lm.StartEveryHour(func(ctx context.Context) {
		// hourly job at hh:00:02
	})

	lm.StartDailyAt(17, 0, 0, func(ctx context.Context) {
		// daily job at 17:00:00
	})

	// Block until a shutdown signal is received
	<-ctx.Done()

	// Optional: give loops a short window to finish in-flight work
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	// Wait for loops to stop; Stop() is already deferred, but we can call explicitly here
	lm.Stop()

	// If additional services need shutdown, coordinate them here using shutdownCtx
	_ = shutdownCtx
}
