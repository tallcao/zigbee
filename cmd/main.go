package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"zigbee/app"
	"zigbee/config"
	"zigbee/database"
	"zigbee/services"
)

func main() {

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Config error:", err)
	}

	database.ConnectDB(cfg)
	// defer database.DB.Close()

	services.InitNats(cfg.NatsUrl)
	// defer services.NC.Drain()

	if err := services.InitMqttService("zigbee-hub", cfg.MqttBroker, cfg.MqttUser, cfg.MqttPassword); err != nil {
		log.Fatal("Failed to initialize SparkplugService:", err)
	}

	zigbeeHub, err := app.NewZigbeeHub(services.DefaultMqttService, services.NC, database.DB)

	if err != nil {
		log.Fatal("Failed to initialize zigbee hub service", err)
	}

	zigbeeHub.Start()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Received shutdown signal, shutting down...")

	services.GetMqttService().Stop()
	services.NC.Drain()
	database.DB.Close()

	log.Println("Shutdown complete.")
}
