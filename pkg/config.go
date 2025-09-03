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
	SFC_API       string
	SFC_CLON      string
	SFC_DB_STATUS string
	MESSAGE_DIR   string
	WS_ADD        string
	WS_PORT       string
	LOG_DIR       string
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
			SFC_API:       getEnv("SFC_API", ""),
			SFC_CLON:      getEnv("SFC_CLON", ""),
			LOG_DIR:       getEnv("LOG_DIR", "logs"),
			SFC_DB_STATUS: getEnv("SFC_DB_STATUS", ""),
			MESSAGE_DIR:   getEnv("MESSAGE_DIR", "broadcast_messages"),
			WS_ADD:        getEnv("WS_ADD", "localhost"),
			WS_PORT:       getEnv("WS_PORT", "8081"),
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
