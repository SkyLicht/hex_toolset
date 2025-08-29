package pkg

import (
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	SFC_API               string
	SFC_CLON              string
	SFC_DB_STATUS         string
	BROADCAST_MESSAGE_DIR string
	BROADCAST_WS_ADDR     string
	LOG_DIR               string
}

var (
	config *Config
	once   sync.Once
)

// GetConfig returns a singleton instance of the configuration
func GetConfig() *Config {
	once.Do(func() {
		// Load .env file if it exists
		err := godotenv.Load()
		if err != nil {
			log.Println("Warning: .env file not found, using default values")
		}

		config = &Config{
			// Use environment variables or default values
			SFC_API:               getEnv("SFC_API", ""),
			SFC_CLON:              getEnv("SFC_CLON", ""),
			LOG_DIR:               getEnv("LOG_DIR", "logs"),
			SFC_DB_STATUS:         getEnv("SFC_DB_STATUS", ""),
			BROADCAST_MESSAGE_DIR: getEnv("BROADCAST_MESSAGE_DIR", "broadcast_messages"),
			BROADCAST_WS_ADDR:     getEnv("BROADCAST_WS_ADDR", ":8081"),
		}

		log.Printf("Configuration loaded: %+v", config)
	})

	return config
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvAsInt gets an environment variable as int or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return defaultValue
}
