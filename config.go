// config.go
package main

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config содержит конфигурацию приложения
type Config struct {
	TelegramToken string
	OpenAIToken   string
}

// LoadConfig загружает конфигурацию из переменных окружения или .env файла
func LoadConfig() (*Config, error) {
	// Загрузка из .env файла
	if err := godotenv.Load(); err != nil {
		log.Printf("Предупреждение: Не удалось загрузить .env файл: %v", err)
		// Продолжаем работу - возможно, переменные окружения уже установлены
	}

	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
		return nil, errors.New("TELEGRAM_TOKEN не установлен")
	}

	openAIToken := os.Getenv("OPENAI_TOKEN")
	if openAIToken == "" {
		return nil, errors.New("OPENAI_TOKEN не установлен")
	}

	// Для локальной разработки - опционально используем webhook secret
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhookSecret == "" {
		log.Printf("ВНИМАНИЕ: STRIPE_WEBHOOK_SECRET не установлен, подписи webhook не будут проверяться")
	}

	// Печатаем порт для отладки
	port := os.Getenv("PORT")
	if port == "" {
		port = "4242"
		log.Printf("Используется порт по умолчанию: %s", port)
	} else {
		log.Printf("Используется указанный порт: %s", port)
	}

	return &Config{
		TelegramToken: telegramToken,
		OpenAIToken:   openAIToken,
	}, nil
}
