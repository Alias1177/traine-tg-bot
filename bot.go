// bot.go
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляет телеграм бота
type Bot struct {
	api          *tgbotapi.BotAPI
	openAIClient *OpenAIClient
	sessions     map[int64]*UserSession
	mutex        sync.RWMutex
}

// NewBot создает нового телеграм бота
func NewBot(token string, openAIClient *OpenAIClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Авторизован бот: %s", api.Self.UserName)

	// Инициализация Stripe
	InitStripe()

	return &Bot{
		api:          api,
		openAIClient: openAIClient,
		sessions:     make(map[int64]*UserSession),
		mutex:        sync.RWMutex{},
	}, nil
}

// Start запускает обработку сообщений
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("Бот начал прослушивание сообщений")
	for update := range updates {
		if update.Message != nil {
			go b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
		}
	}
}

// getSession возвращает сессию пользователя
func (b *Bot) getSession(userID int64) *UserSession {
	b.mutex.RLock()
	session, exists := b.sessions[userID]
	b.mutex.RUnlock()

	if !exists {
		session = NewUserSession(userID)
		b.mutex.Lock()
		b.sessions[userID] = session
		b.mutex.Unlock()
		log.Printf("Создана новая сессия для пользователя %d", userID)
	}

	return session
}

// saveSession сохраняет сессию пользователя
func (b *Bot) saveSession(userID int64, session *UserSession) {
	b.mutex.Lock()
	b.sessions[userID] = session
	b.mutex.Unlock()
	log.Printf("Сохранена сессия для пользователя %d в состоянии %d", userID, session.State)
}

// handleMessage обрабатывает входящие сообщения
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	log.Printf("Получено сообщение от %s (%d): %s", message.From.UserName, message.From.ID, message.Text)

	// Обработка специальных команд
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			// Создаем новую сессию
			session := NewUserSession(message.From.ID)
			b.saveSession(message.From.ID, session)

			// Начинаем диалог
			response, _ := session.ProcessInput("")
			msg := tgbotapi.NewMessage(message.Chat.ID, response)
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "help":
			msg := tgbotapi.NewMessage(message.Chat.ID, "Я помогу создать персональную программу тренировок на основе ваших данных. Используйте /start чтобы начать.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "pay":
			session := b.getSession(message.From.ID)
			if session.State != StatePayment {
				msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, сначала заполните информацию о себе с помощью команды /start")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Ошибка отправки сообщения: %v", err)
				}
				return
			}

			response, err := session.ProcessInput("/pay")
			msg := tgbotapi.NewMessage(message.Chat.ID, response)
			_, err = b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "complete_payment":
			// Отладочная команда для ручного завершения оплаты
			if os.Getenv("ENABLE_DEBUG_COMMANDS") == "true" {
				session := b.getSession(message.From.ID)
				if session.State != StatePayment {
					msg := tgbotapi.NewMessage(message.Chat.ID, "Эта команда работает только если вы находитесь на этапе оплаты")
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Ошибка отправки сообщения: %v", err)
					}
					return
				}

				// Эмулируем успешную оплату
				sessionID := ManuallyCompletePayment(message.From.ID)
				err := b.ProcessPaymentWebhook(sessionID)
				if err != nil {
					msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Ошибка при эмуляции оплаты: %v", err))
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Ошибка отправки сообщения: %v", err)
					}
				}
				return
			}

			// Если отладочные команды отключены, показываем обычную подсказку
			msg := tgbotapi.NewMessage(message.Chat.ID, "Неизвестная команда. Используйте /help для получения справки.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "get_plan", "plan":
			session := b.getSession(message.From.ID)
			if session.State != StateComplete {
				msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, сначала заполните информацию о себе и оплатите услугу с помощью команды /start")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Ошибка отправки сообщения: %v", err)
				}
				return
			}

			// Отправляем уведомление, что начинаем генерацию
			msg := tgbotapi.NewMessage(message.Chat.ID, "Генерирую вашу персональную программу тренировок...")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}

			// Генерируем и отправляем план тренировок
			err = b.sendTrainingPlan(message.Chat.ID, session)
			if err != nil {
				log.Printf("Ошибка отправки плана тренировок: %v", err)
				errorMsg := tgbotapi.NewMessage(message.Chat.ID, "Произошла ошибка при генерации программы тренировок. Пожалуйста, попробуйте позже.")
				_, _ = b.api.Send(errorMsg)
			}
			return
		}
	}

	// Обработка обычных сообщений через сессию
	session := b.getSession(message.From.ID)
	response, err := session.ProcessInput(message.Text)

	// Если это завершенная сессия и сообщение не команда, генерируем ответ GPT
	if session.State == StateComplete && !message.IsCommand() {
		chatAction := tgbotapi.NewChatAction(message.Chat.ID, tgbotapi.ChatTyping)
		_, err := b.api.Request(chatAction)
		if err != nil {
			log.Printf("Ошибка отправки статуса 'печатает': %v", err)
		}

		// Формируем запрос с данными пользователя
		userDataPrompt := fmt.Sprintf("Данные пользователя:\n%s\n\nСообщение пользователя: %s",
			session.Data.String(), message.Text)

		gptResponse, err := b.openAIClient.GetCompletion(userDataPrompt)
		if err != nil {
			log.Printf("Ошибка при получении ответа от OpenAI: %v", err)

			errorMessage := "Произошла ошибка при обращении к OpenAI."
			if strings.Contains(err.Error(), "429") {
				errorMessage = "Превышен лимит запросов к OpenAI. Попробуйте позже."
			}

			msg := tgbotapi.NewMessage(message.Chat.ID, errorMessage)
			_, _ = b.api.Send(msg)
			return
		}

		msg := tgbotapi.NewMessage(message.Chat.ID, gptResponse)
		_, err = b.api.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки ответа: %v", err)
		}
		return
	}

	// Отправляем обычный ответ из процесса диалога
	if err != nil {
		log.Printf("Ошибка обработки ввода: %v", err)
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, response)
	_, err = b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}

	// Сохраняем обновленную сессию
	b.saveSession(message.From.ID, session)
}

// handleCallback обрабатывает callback запросы (для кнопок)
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	// Пока у нас нет обработки callback, но здесь можно будет добавить логику
	// для обработки результатов платежа, если нужно
	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))
}

// sendTrainingPlan генерирует и отправляет план тренировок
func (b *Bot) sendTrainingPlan(chatID int64, session *UserSession) error {
	// Отправляем статус "печатает"
	chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, err := b.api.Request(chatAction)
	if err != nil {
		log.Printf("Ошибка отправки статуса 'печатает': %v", err)
	}

	// Генерируем персональный план тренировок
	userDataJSON := session.Data.String()
	log.Printf("Подготовка запроса к GPT для чата %d с данными: %s", chatID, userDataJSON)

	prompt := fmt.Sprintf(`Создай подробную персональную программу тренировок на 2 недели на основе следующих данных пользователя:
%s

Программа должна включать:
1. Недельный план тренировок с указанием дней, типов тренировок и продолжительности
2. Подробное описание каждой тренировки с упражнениями, подходами и повторениями
3. Рекомендации по питанию
4. Рекомендации по отслеживанию прогресса
5. Дополнительные советы с учетом персональных данных пользователя

Учти наличие диабета и адаптируй программу соответствующим образом.`, userDataJSON)

	log.Printf("Отправка запроса к OpenAI для чата %d", chatID)

	// Получаем ответ от GPT
	trainingPlan, err := b.openAIClient.GetCompletion(prompt)
	if err != nil {
		log.Printf("Ошибка при получении ответа от OpenAI: %v", err)
		return err
	}

	log.Printf("Получен ответ от OpenAI для чата %d (длина: %d символов)", chatID, len(trainingPlan))

	// Отправляем план тренировок пользователю
	planMsg := tgbotapi.NewMessage(chatID, trainingPlan)
	_, err = b.api.Send(planMsg)
	if err != nil {
		log.Printf("Ошибка отправки плана тренировок: %v", err)
		return err
	}
	log.Printf("План тренировок успешно отправлен для чата %d", chatID)

	// Отправляем дополнительное сообщение с инструкциями
	followupMsg := tgbotapi.NewMessage(chatID, "Вот ваша персональная программа тренировок! Теперь вы можете задавать мне вопросы по программе или попросить уточнить любую часть программы.")
	_, err = b.api.Send(followupMsg)
	if err != nil {
		log.Printf("Ошибка отправки финального сообщения: %v", err)
	} else {
		log.Printf("Финальное сообщение успешно отправлено для чата %d", chatID)
	}

	return nil
}

// ProcessPaymentWebhook обрабатывает webhook от Stripe
func (b *Bot) ProcessPaymentWebhook(sessionID string) error {
	log.Printf("Обработка webhook от Stripe для сессии: %s", sessionID)

	success, userIDStr, err := VerifyPayment(sessionID)
	if err != nil {
		log.Printf("Ошибка проверки платежа: %v", err)
		return err
	}

	if !success {
		log.Printf("Платеж не завершен для сессии: %s", sessionID)
		return fmt.Errorf("платеж не завершен")
	}

	// Конвертируем ID пользователя из строки в int64
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		log.Printf("Ошибка конвертации ID пользователя '%s': %v", userIDStr, err)
		return err
	}

	log.Printf("Платеж успешно подтвержден для пользователя: %d", userID)

	// Получаем сессию пользователя
	session := b.getSession(userID)

	// Обновляем статус сессии
	session.SetPaymentCompleted(sessionID)
	b.saveSession(userID, session) // Сохраняем сессию после обновления!
	log.Printf("Статус сессии пользователя %d обновлен как оплаченный", userID)

	// Отправляем уведомление пользователю об успешном платеже
	msg := tgbotapi.NewMessage(userID, "🎉 Оплата успешно завершена! Генерирую вашу персональную программу тренировок...")
	_, err = b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	} else {
		log.Printf("Отправлено уведомление об успешной оплате пользователю %d", userID)
	}

	// Добавляем небольшую задержку перед отправкой плана тренировок
	time.Sleep(2 * time.Second)

	// Отправляем план тренировок
	err = b.sendTrainingPlan(userID, session)
	if err != nil {
		log.Printf("Ошибка при отправке плана тренировок: %v", err)
		// Отправляем сообщение об ошибке пользователю
		errorMsg := tgbotapi.NewMessage(userID, "Произошла ошибка при генерации плана тренировок. Пожалуйста, используйте команду /plan чтобы получить план.")
		_, _ = b.api.Send(errorMsg)
	}

	return nil
}
