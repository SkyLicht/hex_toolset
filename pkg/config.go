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
	SFC_CLON string
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
			SFC_CLON: getEnv("SFC_CLON", ""),
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
