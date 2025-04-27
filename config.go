// config.go
package main

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config contains application configuration
type Config struct {
	TelegramToken string
	OpenAIToken   string
}

// LoadConfig loads configuration from environment variables or .env file
func LoadConfig() (*Config, error) {
	// Load from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Failed to load .env file: %v", err)
		// Continue - environment variables may already be set
	}

	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
		return nil, errors.New("TELEGRAM_TOKEN not set")
	}

	openAIToken := os.Getenv("OPENAI_TOKEN")
	if openAIToken == "" {
		return nil, errors.New("OPENAI_TOKEN not set")
	}

	// For local development - optionally use webhook secret
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhookSecret == "" {
		log.Printf("WARNING: STRIPE_WEBHOOK_SECRET not set, webhook signatures will not be verified")
	}

	// Print port for debugging
	port := os.Getenv("PORT")
	if port == "" {
		port = "4242"
		log.Printf("Using default port: %s", port)
	} else {
		log.Printf("Using specified port: %s", port)
	}

	return &Config{
		TelegramToken: telegramToken,
		OpenAIToken:   openAIToken,
	}, nil
}
