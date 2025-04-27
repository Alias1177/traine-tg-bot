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

// Глобальная переменная для отслеживания обрабатываемых обновлений
var processedUpdates = make(map[int]bool)
var processedMutex sync.RWMutex

// Bot представляет телеграм бота
type Bot struct {
	api          *tgbotapi.BotAPI
	openAIClient *OpenAIClient
	sessions     map[int64]*UserSession
	mutex        sync.RWMutex
	// Для отслеживания последней команды /start для каждого юзера
	lastStartTime map[int64]time.Time
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
		api:           api,
		openAIClient:  openAIClient,
		sessions:      make(map[int64]*UserSession),
		mutex:         sync.RWMutex{},
		lastStartTime: make(map[int64]time.Time),
	}, nil
}

// Start запускает обработку сообщений
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("Бот начал прослушивание сообщений")
	for update := range updates {
		// Проверка на дубликаты обновлений
		processedMutex.RLock()
		_, exists := processedUpdates[update.UpdateID]
		processedMutex.RUnlock()

		if exists {
			log.Printf("Пропуск дублирующего обновления ID: %d", update.UpdateID)
			continue
		}

		// Помечаем обновление как обработанное
		processedMutex.Lock()
		processedUpdates[update.UpdateID] = true
		processedMutex.Unlock()

		// Очистка старых обновлений каждые 100 сообщений
		if len(processedUpdates) > 100 {
			go b.cleanOldUpdates()
		}

		if update.Message != nil {
			go b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go b.handleCallback(update.CallbackQuery)
		}
	}
}

// cleanOldUpdates удаляет старые записи из кэша обработанных обновлений
func (b *Bot) cleanOldUpdates() {
	processedMutex.Lock()
	defer processedMutex.Unlock()

	// Оставляем последние 50 обновлений
	if len(processedUpdates) > 50 {
		processedUpdates = make(map[int]bool)
		log.Printf("Кэш обработанных обновлений очищен")
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

// sendMessageWithKeyboard отправляет сообщение с клавиатурой
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard *tgbotapi.InlineKeyboardMarkup) (int, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	if keyboard != nil {
		msg.ReplyMarkup = keyboard
	}

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		return 0, err
	}

	return sentMsg.MessageID, nil
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	chatID := callback.Message.Chat.ID

	log.Printf("Получен callback от %s (%d): %s", callback.From.UserName, userID, callback.Data)

	// Сначала ответим на коллбэк, чтобы убрать часы загрузки
	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

	// Получаем сессию пользователя
	session := b.getSession(userID)

	// Проверяем дублирование callback
	if session.CheckDuplicateCallback(callback.Data) {
		log.Printf("Пропуск дублирующего callback: %s от пользователя %d", callback.Data, userID)
		return
	}

	// Особая обработка для кнопки "pay"
	if callback.Data == "pay" {
		// Создаем ссылку для оплаты
		paymentURL, err := CreatePayment(userID)
		if err != nil {
			log.Printf("Ошибка создания ссылки для оплаты: %v", err)
			errorMsg := fmt.Sprintf("Произошла ошибка при создании платежа: %v", err)
			b.api.Send(tgbotapi.NewMessage(chatID, errorMsg))
			return
		}

		// Отправляем пользователю ссылку
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Для оплаты перейдите по ссылке: %s", paymentURL))
		_, err = b.api.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки ссылки для оплаты: %v", err)
		}
		return
	}

	// Получаем человекочитаемое представление выбора для отображения в сообщении
	choiceText := getUserFriendlyChoice(callback.Data)

	// Обрабатываем нажатие кнопки
	response, err := session.ProcessButtonCallback(callback.Data)
	if err != nil {
		log.Printf("Ошибка обработки callback: %v", err)
		b.api.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Произошла ошибка. Попробуйте ещё раз. (%v)", err)))
		return
	}

	// Если это команда /pay, обрабатываем её отдельно
	if response == "/pay" {
		paymentLink, err := CreatePayment(userID)
		if err != nil {
			errorMsg := fmt.Sprintf("Произошла ошибка при создании платежа: %v", err)
			b.api.Send(tgbotapi.NewMessage(chatID, errorMsg))
			return
		}

		payMsg := fmt.Sprintf("Для оплаты перейдите по ссылке: %s", paymentLink)
		b.api.Send(tgbotapi.NewMessage(chatID, payMsg))
		b.saveSession(userID, session)
		return
	}

	// Удаляем старую клавиатуру у предыдущего сообщения
	if callback.Message != nil {
		editMsg := tgbotapi.NewEditMessageText(
			chatID,
			callback.Message.MessageID,
			callback.Message.Text+"\n\n✅ Выбрано: "+choiceText,
		)
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{},
		}
		_, err := b.api.Send(editMsg)
		if err != nil {
			log.Printf("Ошибка при редактировании сообщения: %v", err)
		}
	}

	// Отправляем следующий вопрос с клавиатурой
	keyboard := session.GetKeyboardForState()
	messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
	if err != nil {
		log.Printf("Ошибка отправки сообщения с клавиатурой: %v", err)
	} else {
		session.LastMessageID = messageID
	}

	// Сохраняем обновленную сессию
	b.saveSession(userID, session)
}

// getUserFriendlyChoice возвращает удобное для пользователя представление выбора
func getUserFriendlyChoice(data string) string {
	if len(data) < 4 {
		return data
	}

	var prefix, value string

	// Определяем префикс и значение
	if data[:4] == "sex:" {
		prefix = CallbackSex
		value = data[4:]
	} else if data[:4] == "dia:" {
		prefix = CallbackDiabetes
		value = data[4:]
	} else if data[:4] == "lvl:" {
		prefix = CallbackLevel
		value = data[4:]
	} else if data[:4] == "gol:" {
		prefix = CallbackGoal
		value = data[4:]
	} else if data[:4] == "typ:" {
		prefix = CallbackType
		value = data[4:]
	} else {
		return data
	}

	switch prefix {
	case CallbackSex:
		return map[string]string{
			"male":   "Мужской",
			"female": "Женский",
		}[value]

	case CallbackDiabetes:
		return map[string]string{
			"yes": "Да",
			"no":  "Нет",
		}[value]

	case CallbackLevel:
		return map[string]string{
			"beginner":     "Начинающий",
			"intermediate": "Средний",
			"advanced":     "Продвинутый",
		}[value]

	case CallbackGoal:
		return map[string]string{
			"weight_loss": "Похудение",
			"muscle_gain": "Набор массы",
			"maintenance": "Поддержание формы",
			"endurance":   "Улучшение выносливости",
		}[value]

	case CallbackType:
		return map[string]string{
			"strength": "Силовые",
			"cardio":   "Кардио",
			"mixed":    "Смешанные",
			"yoga":     "Йога",
			"pilates":  "Пилатес",
			"other":    "Другое",
		}[value]
	}

	return data
}

// checkStartCommand проверяет, можно ли обработать команду /start
func (b *Bot) checkStartCommand(userID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	lastTime, exists := b.lastStartTime[userID]
	now := time.Now()

	// Если команда /start вызвана впервые или прошло более 5 секунд с предыдущего вызова
	if !exists || now.Sub(lastTime) > 5*time.Second {
		b.lastStartTime[userID] = now
		return true
	}

	return false
}

// handleMessage обрабатывает входящие сообщения
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID
	chatID := message.Chat.ID

	log.Printf("Получено сообщение от %s (%d): %s", message.From.UserName, userID, message.Text)

	// Получаем сессию пользователя
	session := b.getSession(userID)

	// Проверяем лимит сообщений
	if !session.IncrementMessageCount() {
		msg := tgbotapi.NewMessage(chatID, "Вы достигли лимита сообщений. Пожалуйста, попробуйте позже.")
		_, err := b.api.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки сообщения о лимите: %v", err)
		}
		return
	}

	// Обработка специальных команд
	if message.IsCommand() {
		// Проверяем на дублирование команды
		if session.CheckDuplicateCommand(message.Text) {
			log.Printf("Пропуск дублирующей команды: %s от пользователя %d", message.Text, userID)
			return
		}

		switch message.Command() {
		case "start":
			// Дополнительная проверка на дублирование /start
			if !b.checkStartCommand(userID) {
				log.Printf("Пропуск дублирующей команды /start от пользователя %d", userID)
				return
			}

			// Создаем новую сессию
			session = NewUserSession(userID)
			b.saveSession(userID, session)

			// Начинаем диалог
			response, _ := session.ProcessInput("")
			keyboard := session.GetKeyboardForState() // Получаем клавиатуру для текущего состояния

			messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			} else {
				session.LastMessageID = messageID
				b.saveSession(userID, session)
			}
			return

		case "help":
			msg := tgbotapi.NewMessage(chatID, "Я помогу создать персональную программу тренировок на основе ваших данных. Используйте /start чтобы начать.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "pay":
			if session.State != StatePayment {
				msg := tgbotapi.NewMessage(chatID, "Пожалуйста, сначала заполните информацию о себе с помощью команды /start")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Ошибка отправки сообщения: %v", err)
				}
				return
			}

			response, err := session.ProcessInput("/pay")
			msg := tgbotapi.NewMessage(chatID, response)
			_, err = b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "complete_payment":
			// Отладочная команда для ручного завершения оплаты
			if os.Getenv("ENABLE_DEBUG_COMMANDS") == "true" {
				if session.State != StatePayment {
					msg := tgbotapi.NewMessage(chatID, "Эта команда работает только если вы находитесь на этапе оплаты")
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Ошибка отправки сообщения: %v", err)
					}
					return
				}

				// Эмулируем успешную оплату
				sessionID := ManuallyCompletePayment(userID)
				err := b.ProcessPaymentWebhook(sessionID)
				if err != nil {
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при эмуляции оплаты: %v", err))
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Ошибка отправки сообщения: %v", err)
					}
				}
				return
			}

			// Если отладочные команды отключены, показываем обычную подсказку
			msg := tgbotapi.NewMessage(chatID, "Неизвестная команда. Используйте /help для получения справки.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}
			return

		case "get_plan", "plan":
			if session.State != StateComplete {
				msg := tgbotapi.NewMessage(chatID, "Пожалуйста, сначала заполните информацию о себе и оплатите услугу с помощью команды /start")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Ошибка отправки сообщения: %v", err)
				}
				return
			}

			// Отправляем уведомление, что начинаем генерацию
			msg := tgbotapi.NewMessage(chatID, "Генерирую вашу персональную программу тренировок...")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}

			// Генерируем и отправляем план тренировок
			err = b.sendTrainingPlan(chatID, session)
			if err != nil {
				log.Printf("Ошибка отправки плана тренировок: %v", err)
				errorMsg := tgbotapi.NewMessage(chatID, "Произошла ошибка при генерации программы тренировок. Пожалуйста, попробуйте позже.")
				_, _ = b.api.Send(errorMsg)
			}
			return
		}
	} else {
		// Для не-команд проверяем дублирование только для состояния завершено
		if session.State == StateComplete && session.CheckDuplicateCommand(message.Text) {
			log.Printf("Пропуск дублирующего сообщения от пользователя %d", userID)
			return
		}
	}

	// Обработка обычных сообщений через сессию
	response, err := session.ProcessInput(message.Text)
	if err != nil {
		log.Printf("Ошибка обработки ввода: %v", err)
	}

	// Если это завершенная сессия и сообщение не команда, генерируем ответ GPT
	if session.State == StateComplete && !message.IsCommand() {
		chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
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

			msg := tgbotapi.NewMessage(chatID, errorMessage)
			_, _ = b.api.Send(msg)
			return
		}

		msg := tgbotapi.NewMessage(chatID, gptResponse)
		_, err = b.api.Send(msg)
		if err != nil {
			log.Printf("Ошибка отправки ответа: %v", err)
		}
		return
	}

	// Получаем клавиатуру для текущего состояния
	keyboard := session.GetKeyboardForState()

	// Отправляем сообщение с клавиатурой
	messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
	if err != nil {
		log.Printf("Ошибка отправки сообщения с клавиатурой: %v", err)
	} else {
		session.LastMessageID = messageID
	}

	// Сохраняем обновленную сессию
	b.saveSession(userID, session)
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

	prompt := fmt.Sprintf(`Создай подробную персональную программу тренировок на 1 неделю посчитав индекс тела на основе следующих данных пользователя.И дай минимально 5 тренировок и дополнительно минимул 3 тренировки на живот:
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

	// Добавляем кнопки для удобства дальнейшего взаимодействия
	followupMsg := tgbotapi.NewMessage(
		chatID,
		"Вот ваша персональная программа тренировок! Теперь вы можете задавать мне вопросы по программе или попросить уточнить любую часть программы.",
	)

	// Добавляем кнопки подсказки для вопросов
	followupMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Уточнить питание", CallbackAsk+"nutrition"),
			tgbotapi.NewInlineKeyboardButtonData("Уточнить упражнения", CallbackAsk+"exercises"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Как отслеживать прогресс", CallbackAsk+"progress"),
			tgbotapi.NewInlineKeyboardButtonData("Что делать при диабете", CallbackAsk+"diabetes"),
		),
	)

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
