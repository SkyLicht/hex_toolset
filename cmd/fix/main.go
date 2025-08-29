package main

import (
	"context"
	"fmt"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/logger"
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

	// Create custom logger named sfc_loader
	lgr, _ := logger.New(
		logger.WithName("sfc_loader"),
		logger.WithFilePattern("{name}.log"),
		logger.WithConsole(true),
	)

	defer func() {
		// Always attempt to close DB at the end
		if err := db.GetInstance().CloseDB(); err != nil {
			if lgr != nil {
				lgr.Errorf("error closing database: %v", err)
			} else {
				fmt.Printf("error closing database: %v\n", err)
			}
		}
		if lgr != nil {
			_ = lgr.Close()
		}
	}()

	if err := db.GetInstance().InitDefault(ctx); err != nil {
		if lgr != nil {
			lgr.Errorf("Error initializing database: %v", err)
		} else {
			fmt.Printf("Error initializing database: %v\n", err)
		}
		return
	}
	if err := db.GetInstance().HealthCheck(ctx); err != nil {
		if lgr != nil {
			lgr.Errorf("Error checking database health: %v", err)
		} else {
			fmt.Printf("Error checking database health: %v\n", err)
		}
		return
	}
	if lgr != nil {
		lgr.Infof("DB initialized")
	} else {
		fmt.Println("DB initialized")
	}

	// Initialize managers with the long-lived context
	sfcManager := managers.NewSFCAPIManager(&ctx)
	if sfcManager == nil {
		if lgr != nil {
			lgr.Errorf("failed to create SFC manager")
		}
		return
	}

	args := os.Args
	if len(args) < 2 {
		fmt.Println("usage:")
		fmt.Println("  fix load_day YYYY-MM-DD")
		fmt.Println("  fix load_days YYYY-MM-DD YYYY-MM-DD")
		fmt.Println("  fix load_hour \"YYYY-MM-DD HH\"")
		return
	}

	switch args[1] {
	case "load_day":
		if len(args) != 3 {
			fmt.Println("usage: fix load_day YYYY-MM-DD")
			return
		}
		date := args[2]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			fmt.Printf("invalid date %q, expected YYYY-MM-DD: %v\n", date, err)
			return
		}
		if err := sfcManager.LoadDay(ctx, date); err != nil {
			if lgr != nil {
				lgr.Errorf("load_day failed: %v", err)
			} else {
				fmt.Printf("load_day failed: %v\n", err)
			}
			return
		}
		if lgr != nil {
			lgr.Infof("load_day completed for %s", date)
		}

	case "load_days":
		if len(args) != 4 {
			fmt.Println("usage: fix load_days YYYY-MM-DD YYYY-MM-DD")
			return
		}
		start := args[2]
		end := args[3]
		startT, err1 := time.Parse("2006-01-02", start)
		endT, err2 := time.Parse("2006-01-02", end)
		if err1 != nil || err2 != nil {
			if err1 != nil {
				fmt.Printf("invalid start date %q: %v\n", start, err1)
			}
			if err2 != nil {
				fmt.Printf("invalid end date %q: %v\n", end, err2)
			}
			return
		}
		if endT.Before(startT) {
			fmt.Printf("end date %s is before start date %s\n", end, start)
			return
		}
		if err := sfcManager.LoadRangeOfDays(ctx, start, end); err != nil {
			if lgr != nil {
				lgr.Errorf("load_days failed: %v", err)
			} else {
				fmt.Printf("load_days failed: %v\n", err)
			}
			return
		}
		if lgr != nil {
			lgr.Infof("load_days completed for %s .. %s", start, end)
		}

	case "load_hour":
		if len(args) != 3 {
			fmt.Println("usage: fix load_hour \"YYYY-MM-DD HH\"")
			return
		}
		hourStr := args[2]
		if _, err := time.Parse("2006-01-02 15", hourStr); err != nil {
			fmt.Printf("invalid hour %q, expected \"YYYY-MM-DD HH\": %v\n", hourStr, err)
			return
		}
		if err := sfcManager.LoadHour(hourStr); err != nil {
			if lgr != nil {
				lgr.Errorf("load_hour failed: %v", err)
			} else {
				fmt.Printf("load_hour failed: %v\n", err)
			}
			return
		}
		if lgr != nil {
			lgr.Infof("load_hour completed for %s", hourStr)
		}

	default:
		fmt.Println("unknown command. usage:")
		fmt.Println("  fix load_day YYYY-MM-DD")
		fmt.Println("  fix load_days YYYY-MM-DD YYYY-MM-DD")
		fmt.Println("  fix load_hour \"YYYY-MM-DD HH\"")
		return
	}

	// Wait briefly to allow any async logs to flush, or respond to signals
	select {
	case <-ctx.Done():
		// graceful shutdown triggered
	case <-time.After(100 * time.Millisecond):
	}
}
