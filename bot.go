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

// Global variable for tracking processed updates
var processedUpdates = make(map[int]bool)
var processedMutex sync.RWMutex

// Bot represents a telegram bot
type Bot struct {
	api          *tgbotapi.BotAPI
	openAIClient *OpenAIClient
	sessions     map[int64]*UserSession
	mutex        sync.RWMutex
	// For tracking the last /start command for each user
	lastStartTime map[int64]time.Time
}

// NewBot creates a new telegram bot
func NewBot(token string, openAIClient *OpenAIClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized bot: %s", api.Self.UserName)

	// Initialize Stripe
	InitStripe()

	return &Bot{
		api:           api,
		openAIClient:  openAIClient,
		sessions:      make(map[int64]*UserSession),
		mutex:         sync.RWMutex{},
		lastStartTime: make(map[int64]time.Time),
	}, nil
}

// Start begins processing messages
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Printf("Bot started listening for messages")
	for update := range updates {
		// Check for duplicate updates
		processedMutex.RLock()
		_, exists := processedUpdates[update.UpdateID]
		processedMutex.RUnlock()

		if exists {
			log.Printf("Skipping duplicate update ID: %d", update.UpdateID)
			continue
		}

		// Mark update as processed
		processedMutex.Lock()
		processedUpdates[update.UpdateID] = true
		processedMutex.Unlock()

		// Clean old updates every 100 messages
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

// cleanOldUpdates removes old entries from the processed updates cache
func (b *Bot) cleanOldUpdates() {
	processedMutex.Lock()
	defer processedMutex.Unlock()

	// Keep only the last 50 updates
	if len(processedUpdates) > 50 {
		processedUpdates = make(map[int]bool)
		log.Printf("Processed updates cache cleared")
	}
}

// getSession returns the user's session
func (b *Bot) getSession(userID int64) *UserSession {
	b.mutex.RLock()
	session, exists := b.sessions[userID]
	b.mutex.RUnlock()

	if !exists {
		session = NewUserSession(userID)
		b.mutex.Lock()
		b.sessions[userID] = session
		b.mutex.Unlock()
		log.Printf("Created new session for user %d", userID)
	}

	return session
}

// saveSession saves the user's session
func (b *Bot) saveSession(userID int64, session *UserSession) {
	b.mutex.Lock()
	b.sessions[userID] = session
	b.mutex.Unlock()
	log.Printf("Saved session for user %d in state %d", userID, session.State)
}

// sendMessageWithKeyboard sends a message with a keyboard
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

	log.Printf("Received callback from %s (%d): %s", callback.From.UserName, userID, callback.Data)

	// First, respond to the callback to remove the loading clock
	b.api.Request(tgbotapi.NewCallback(callback.ID, ""))

	// Get user session
	session := b.getSession(userID)

	// Check for duplicate callback
	if session.CheckDuplicateCallback(callback.Data) {
		log.Printf("Skipping duplicate callback: %s from user %d", callback.Data, userID)
		return
	}

	// Special handling for "pay" button
	if callback.Data == "pay" {
		// Create payment link
		paymentURL, err := CreatePayment(userID)
		if err != nil {
			log.Printf("Error creating payment link: %v", err)
			errorMsg := fmt.Sprintf("An error occurred while creating payment: %v", err)
			b.api.Send(tgbotapi.NewMessage(chatID, errorMsg))
			return
		}

		// Send the link to the user
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("To make a payment, follow this link: %s", paymentURL))
		_, err = b.api.Send(msg)
		if err != nil {
			log.Printf("Error sending payment link: %v", err)
		}
		return
	}

	// Get human-readable representation of the choice for display in the message
	choiceText := getUserFriendlyChoice(callback.Data)

	// Process button click
	response, err := session.ProcessButtonCallback(callback.Data)
	if err != nil {
		log.Printf("Error processing callback: %v", err)
		b.api.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("An error occurred. Please try again. (%v)", err)))
		return
	}

	// If this is the /pay command, process it separately
	if response == "/pay" {
		paymentLink, err := CreatePayment(userID)
		if err != nil {
			errorMsg := fmt.Sprintf("An error occurred while creating payment: %v", err)
			b.api.Send(tgbotapi.NewMessage(chatID, errorMsg))
			return
		}

		payMsg := fmt.Sprintf("To make a payment, follow this link: %s", paymentLink)
		b.api.Send(tgbotapi.NewMessage(chatID, payMsg))
		b.saveSession(userID, session)
		return
	}

	// Remove the old keyboard from the previous message
	if callback.Message != nil {
		editMsg := tgbotapi.NewEditMessageText(
			chatID,
			callback.Message.MessageID,
			callback.Message.Text+"\n\n✅ Selected: "+choiceText,
		)
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{},
		}
		_, err := b.api.Send(editMsg)
		if err != nil {
			log.Printf("Error editing message: %v", err)
		}
	}

	// Send the next question with a keyboard
	keyboard := session.GetKeyboardForState()
	messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
	if err != nil {
		log.Printf("Error sending message with keyboard: %v", err)
	} else {
		session.LastMessageID = messageID
	}

	// Save updated session
	b.saveSession(userID, session)
}

// getUserFriendlyChoice returns a user-friendly representation of the choice
func getUserFriendlyChoice(data string) string {
	if len(data) < 4 {
		return data
	}

	var prefix, value string

	// Determine prefix and value
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
			"male":   "Male",
			"female": "Female",
		}[value]

	case CallbackDiabetes:
		return map[string]string{
			"yes": "Yes",
			"no":  "No",
		}[value]

	case CallbackLevel:
		return map[string]string{
			"beginner":     "Beginner",
			"intermediate": "Intermediate",
			"advanced":     "Advanced",
		}[value]

	case CallbackGoal:
		return map[string]string{
			"weight_loss": "Weight Loss",
			"muscle_gain": "Muscle Gain",
			"maintenance": "Maintenance",
			"endurance":   "Endurance Improvement",
		}[value]

	case CallbackType:
		return map[string]string{
			"strength": "Strength",
			"cardio":   "Cardio",
			"mixed":    "Mixed",
			"yoga":     "Yoga",
			"pilates":  "Pilates",
			"other":    "Other",
		}[value]
	}

	return data
}

// checkStartCommand checks if the /start command can be processed
func (b *Bot) checkStartCommand(userID int64) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	lastTime, exists := b.lastStartTime[userID]
	now := time.Now()

	// If the /start command is called for the first time or more than 5 seconds have passed since the previous call
	if !exists || now.Sub(lastTime) > 5*time.Second {
		b.lastStartTime[userID] = now
		return true
	}

	return false
}

// handleMessage processes incoming messages
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	userID := message.From.ID
	chatID := message.Chat.ID

	log.Printf("Received message from %s (%d): %s", message.From.UserName, userID, message.Text)

	// Get user session
	session := b.getSession(userID)

	// Check message limit
	if !session.IncrementMessageCount() {
		msg := tgbotapi.NewMessage(chatID, "You have reached the message limit. Please try again later.")
		_, err := b.api.Send(msg)
		if err != nil {
			log.Printf("Error sending limit message: %v", err)
		}
		return
	}

	// Process special commands
	if message.IsCommand() {
		// Check for duplicate command
		if session.CheckDuplicateCommand(message.Text) {
			log.Printf("Skipping duplicate command: %s from user %d", message.Text, userID)
			return
		}

		switch message.Command() {
		case "start":
			// Additional check for duplicate /start
			if !b.checkStartCommand(userID) {
				log.Printf("Skipping duplicate command /start from user %d", userID)
				return
			}

			// Create a new session
			session = NewUserSession(userID)
			b.saveSession(userID, session)

			// Start dialog
			response, _ := session.ProcessInput("")
			keyboard := session.GetKeyboardForState() // Get keyboard for current state

			messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			} else {
				session.LastMessageID = messageID
				b.saveSession(userID, session)
			}
			return

		case "help":
			msg := tgbotapi.NewMessage(chatID, "I will help create a personalized workout program based on your data. Use /start to begin.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			}
			return

		case "pay":
			if session.State != StatePayment {
				msg := tgbotapi.NewMessage(chatID, "Please first fill in your information using the /start command")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Error sending message: %v", err)
				}
				return
			}

			response, err := session.ProcessInput("/pay")
			msg := tgbotapi.NewMessage(chatID, response)
			_, err = b.api.Send(msg)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			}
			return

		case "complete_payment":
			// Debug command for manual payment completion
			if os.Getenv("ENABLE_DEBUG_COMMANDS") == "true" {
				if session.State != StatePayment {
					msg := tgbotapi.NewMessage(chatID, "This command only works if you are at the payment stage")
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Error sending message: %v", err)
					}
					return
				}

				// Emulate successful payment
				sessionID := ManuallyCompletePayment(userID)
				err := b.ProcessPaymentWebhook(sessionID)
				if err != nil {
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Error emulating payment: %v", err))
					_, err := b.api.Send(msg)
					if err != nil {
						log.Printf("Error sending message: %v", err)
					}
				}
				return
			}

			// If debug commands are disabled, show standard help
			msg := tgbotapi.NewMessage(chatID, "Unknown command. Use /help for assistance.")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			}
			return

		case "get_plan", "plan":
			if session.State != StateComplete {
				msg := tgbotapi.NewMessage(chatID, "Please first fill in your information and pay for the service using the /start command")
				_, err := b.api.Send(msg)
				if err != nil {
					log.Printf("Error sending message: %v", err)
				}
				return
			}

			// Send notification that we're starting generation
			msg := tgbotapi.NewMessage(chatID, "Generating your personalized workout program...")
			_, err := b.api.Send(msg)
			if err != nil {
				log.Printf("Error sending message: %v", err)
			}

			// Generate and send workout plan
			err = b.sendTrainingPlan(chatID, session)
			if err != nil {
				log.Printf("Error sending workout plan: %v", err)
				errorMsg := tgbotapi.NewMessage(chatID, "An error occurred while generating the workout program. Please try again later.")
				_, _ = b.api.Send(errorMsg)
			}
			return
		}
	} else {
		// For non-commands, check for duplication only if in completed state
		if session.State == StateComplete && session.CheckDuplicateCommand(message.Text) {
			log.Printf("Skipping duplicate message from user %d", userID)
			return
		}
	}

	// Process regular messages through the session
	response, err := session.ProcessInput(message.Text)
	if err != nil {
		log.Printf("Error processing input: %v", err)
	}

	// If this is a completed session and the message is not a command, generate GPT response
	if session.State == StateComplete && !message.IsCommand() {
		chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
		_, err := b.api.Request(chatAction)
		if err != nil {
			log.Printf("Error sending 'typing' status: %v", err)
		}

		// Form request with user data
		userDataPrompt := fmt.Sprintf("User data:\n%s\n\nUser message: %s",
			session.Data.String(), message.Text)

		gptResponse, err := b.openAIClient.GetCompletion(userDataPrompt)
		if err != nil {
			log.Printf("Error getting response from OpenAI: %v", err)

			errorMessage := "An error occurred while communicating with OpenAI."
			if strings.Contains(err.Error(), "429") {
				errorMessage = "Request limit to OpenAI exceeded. Please try again later."
			}

			msg := tgbotapi.NewMessage(chatID, errorMessage)
			_, _ = b.api.Send(msg)
			return
		}

		msg := tgbotapi.NewMessage(chatID, gptResponse)
		_, err = b.api.Send(msg)
		if err != nil {
			log.Printf("Error sending response: %v", err)
		}
		return
	}

	// Get keyboard for current state
	keyboard := session.GetKeyboardForState()

	// Send message with keyboard
	messageID, err := b.sendMessageWithKeyboard(chatID, response, keyboard)
	if err != nil {
		log.Printf("Error sending message with keyboard: %v", err)
	} else {
		session.LastMessageID = messageID
	}

	// Save updated session
	b.saveSession(userID, session)
}

// sendTrainingPlan generates and sends a workout plan
func (b *Bot) sendTrainingPlan(chatID int64, session *UserSession) error {
	// Send "typing" status
	chatAction := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, err := b.api.Request(chatAction)
	if err != nil {
		log.Printf("Error sending 'typing' status: %v", err)
	}

	// Generate personalized workout plan
	userDataJSON := session.Data.String()
	log.Printf("Preparing GPT request for chat %d with data: %s", chatID, userDataJSON)

	prompt := fmt.Sprintf(`Create a detailed personalized workout program for 1 week calculating body index based on the following user data. And give a minimum of 5 workouts plus an additional minimum of 3 ab workouts. Also give some motivation sentence for user.:
%s

The program should include:
1. Weekly workout plan with days, workout types, and duration
2. Detailed description of each workout with exercises, sets, and repetitions
3. Nutrition recommendations
4. Progress tracking recommendations
5. Additional tips considering the user's personal data

Consider the presence of diabetes and adapt the program accordingly.`, userDataJSON)

	log.Printf("Sending request to OpenAI for chat %d", chatID)

	// Get response from GPT
	trainingPlan, err := b.openAIClient.GetCompletion(prompt)
	if err != nil {
		log.Printf("Error getting response from OpenAI: %v", err)
		return err
	}

	log.Printf("Received response from OpenAI for chat %d (length: %d characters)", chatID, len(trainingPlan))

	// Send workout plan to user
	planMsg := tgbotapi.NewMessage(chatID, trainingPlan)
	_, err = b.api.Send(planMsg)
	if err != nil {
		log.Printf("Error sending workout plan: %v", err)
		return err
	}
	log.Printf("Workout plan successfully sent for chat %d", chatID)

	// Add buttons for further interaction
	followupMsg := tgbotapi.NewMessage(
		chatID,
		"Here's your personalized workout program! Now you can ask me questions about the program or request clarification on any part of the program.",
	)

	// Add hint buttons for questions
	followupMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Clarify nutrition", CallbackAsk+"nutrition"),
			tgbotapi.NewInlineKeyboardButtonData("Clarify exercises", CallbackAsk+"exercises"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("How to track progress", CallbackAsk+"progress"),
			tgbotapi.NewInlineKeyboardButtonData("What to do with diabetes", CallbackAsk+"diabetes"),
		),
	)

	_, err = b.api.Send(followupMsg)
	if err != nil {
		log.Printf("Error sending final message: %v", err)
	} else {
		log.Printf("Final message successfully sent for chat %d", chatID)
	}

	return nil
}

// ProcessPaymentWebhook processes webhook from Stripe
func (b *Bot) ProcessPaymentWebhook(sessionID string) error {
	log.Printf("Processing webhook from Stripe for session: %s", sessionID)

	success, userIDStr, err := VerifyPayment(sessionID)
	if err != nil {
		log.Printf("Error verifying payment: %v", err)
		return err
	}

	if !success {
		log.Printf("Payment not completed for session: %s", sessionID)
		return fmt.Errorf("payment not completed")
	}

	// Convert user ID from string to int64
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		log.Printf("Error converting user ID '%s': %v", userIDStr, err)
		return err
	}

	log.Printf("Payment successfully confirmed for user: %d", userID)

	// Get user session
	session := b.getSession(userID)

	// Update session status
	session.SetPaymentCompleted(sessionID)
	b.saveSession(userID, session) // Save session after update!
	log.Printf("User %d session status updated as paid", userID)

	// Send notification to user about successful payment
	msg := tgbotapi.NewMessage(userID, "🎉 Payment successfully completed! Generating your personalized workout program...")
	_, err = b.api.Send(msg)
	if err != nil {
		log.Printf("Error sending message: %v", err)
	} else {
		log.Printf("Sent successful payment notification to user %d", userID)
	}

	// Add a small delay before sending workout plan
	time.Sleep(2 * time.Second)

	// Send workout plan
	err = b.sendTrainingPlan(userID, session)
	if err != nil {
		log.Printf("Error sending workout plan: %v", err)
		// Send error message to user
		errorMsg := tgbotapi.NewMessage(userID, "An error occurred while generating the workout plan. Please use the /plan command to get the plan.")
		_, _ = b.api.Send(errorMsg)
	}

	return nil
}
