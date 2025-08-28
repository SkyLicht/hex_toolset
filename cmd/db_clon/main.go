package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hex_toolset/cmd/db_clon/managers"
)

func main() {
	// Root context that cancels on SIGINT/SIGTERM for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize managers with the long-lived context
	sfcManager := managers.NewSFCAPIManager(&ctx)
	lm := managers.NewLoopsManager(ctx)
	defer lm.Stop() // ensure loops are stopped on exit

	// Start loops (run in parallel)
	lm.StartEveryMinute(func(ctx context.Context, minute time.Time) {
		sfcManager.GetCurrentMinute(minute)
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
