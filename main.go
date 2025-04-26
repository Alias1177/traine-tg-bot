// main.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Загрузка конфигурации
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Инициализация OpenAI клиента
	openAIClient := NewOpenAIClient(config.OpenAIToken)

	// Инициализация и запуск бота
	bot, err := NewBot(config.TelegramToken, openAIClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}

	// Запуск обработки сообщений в отдельной горутине
	go bot.Start()
	fmt.Println("Бот запущен...")

	// Настройка graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Завершение работы бота...")
}
