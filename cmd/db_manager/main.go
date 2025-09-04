package main

import (
	"context"
	"fmt"
	"hex_toolset/pkg/db"
	"hex_toolset/pkg/db/entities"
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
	defer func() {
		if err := db.GetInstance().CloseDB(); err != nil {
			log.Printf("close db error: %v", err)
		}
	}()

	dbInstance := db.GetInstance().GetDB()

	if err := entities.NewRecordManagerEntity(dbInstance).CreateTable(); err != nil {
		log.Fatal(err)
	}
	if err := entities.NewLatestPassManager(dbInstance).CreateTable(); err != nil {
		log.Fatal(err)
	}
	if err := entities.NewLatestGroupManager(dbInstance).CreateTable(); err != nil {
		log.Fatal(err)
	}
	// Create triggers
	if err := (entities.NewTriggersManager(dbInstance)).CreateRecordsPassUpsertTrigger(); err != nil {
		log.Fatal(err)
	}
	if err := (entities.NewTriggersManager(dbInstance)).CreateRecordsGroupUpsertTrigger(); err != nil {
		log.Fatal(err)
	}
	err := db.GetInstance().CloseDB()
	if err != nil {
		return
	}

	fmt.Println("DB end")
}
