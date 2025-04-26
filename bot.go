// bot.go
package main

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляет телеграм бота
type Bot struct {
	api          *tgbotapi.BotAPI
	openAIClient *OpenAIClient
}

// NewBot создает нового телеграм бота
func NewBot(token string, openAIClient *OpenAIClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{
		api:          api,
		openAIClient: openAIClient,
	}, nil
}

// Start запускает обработку сообщений
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go b.handleMessage(update.Message)
	}
}

// handleMessage обрабатывает входящие сообщения
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Игнорируем команду /start
	if message.IsCommand() && message.Command() == "start" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Привет! Я бот, который может общаться с GPT. Просто отправь мне сообщение, и я передам его GPT и верну ответ.")
		_, err := b.api.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки приветствия: %v", err)
		}
		return
	}

	if message.Text == "" {
		return
	}

	// Отправляем уведомление о том, что бот печатает
	chatAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
	_, err := b.api.Request(chatAction) // Используем Request вместо Send
	if err != nil {
		log.Printf("Ошибка отправки статуса 'печатает': %v", err)
	}

	// Получаем ответ от GPT
	response, err := b.openAIClient.GetCompletion(message.Text)
	if err != nil {
		log.Printf("Ошибка при получении ответа от OpenAI: %v", err)

		// Более дружелюбное сообщение об ошибке для пользователя
		errorMessage := "Произошла ошибка при обращении к OpenAI."

		// Проверяем тип ошибки
		if strings.Contains(err.Error(), "429") {
			errorMessage = "Превышен лимит запросов к OpenAI. Проверьте статус подписки и баланс аккаунта."
		} else if strings.Contains(err.Error(), "401") {
			errorMessage = "Ошибка авторизации в OpenAI. Проверьте правильность API ключа."
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, errorMessage)
		_, _ = b.api.Send(msg)
		return
	}

	// Отправляем ответ пользователю
	msg := tgbotapi.NewMessage(message.Chat.ID, response)
	msg.ReplyToMessageID = message.MessageID
	_, err = b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки ответа: %v", err)
	}
}
