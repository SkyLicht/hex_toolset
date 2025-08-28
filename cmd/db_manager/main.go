package main

import (
	"context"
	"fmt"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/db/entity"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Println("DB Manager is running")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := db.GetInstance().InitDefault(ctx); err != nil {
		log.Fatal(err)
	}

	if err := db.GetInstance().HealthCheck(ctx); err != nil {
		log.Fatal(err)
	}

	dbInstance := db.GetInstance().GetDB()

	recordEntity := entity.NewRecordManagerEntity(dbInstance)

	err := recordEntity.CreateTable()
	if err != nil {
		return
	}

	err = db.GetDB().Close()
	if err != nil {
		return
	}
	fmt.Println("DB end")

}
