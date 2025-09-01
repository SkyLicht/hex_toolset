package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/db/entities"
	"hex_toolset/pkg/logger"
	"hex_toolset/pkg/managers"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Root context that cancels on SIGINT/SIGTERM for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create custom logger named sfc_loader
	lgr, _ := logger.New(
		logger.WithName("broadcast_manager"),
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

	entity_manager := entities.NewRecordManagerEntity(db.GetDB())

	hour, err := entity_manager.GetLastHour()
	if err != nil {
		return
	}

	b, err := json.MarshalIndent(hour, "", "  ")
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}

	storeManager, err := managers.NewStoreFileManager()

	_, err = storeManager.SaveWithTimestampWrapped("dddd", "LAST_HOUR", hour)

	fmt.Println(string(b))

}
