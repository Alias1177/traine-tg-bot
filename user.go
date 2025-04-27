// user.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserState represents the state of dialog with the user
type UserState int

const (
	StateInitial UserState = iota
	StateAskSex
	StateAskAge
	StateAskHeight
	StateAskWeight
	StateAskDiabetes
	StateAskLevel
	StateAskGoal
	StateAskType
	StatePayment
	StateComplete
)

// CallbackPrefix - prefixes for callback data
const (
	CallbackSex      = "sex:"
	CallbackDiabetes = "dia:"
	CallbackLevel    = "lvl:"
	CallbackGoal     = "gol:"
	CallbackType     = "typ:"
	CallbackAsk      = "ask:"
)

// UserData structure for storing user data
type UserData struct {
	Sex         string    `json:"Sex"`
	Age         int       `json:"Age"`
	Height      int       `json:"Height"`
	Weight      int       `json:"Weight"`
	Diabetes    string    `json:"Diabetes"`
	Level       string    `json:"Level"`
	FitnessGoal string    `json:"Fitness Goal"`
	FitnessType string    `json:"Fitness Type"`
	PaymentID   string    `json:"payment_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (u *UserData) String() string {
	// For internal purposes (logging, webhook) use JSON
	if os.Getenv("USE_JSON_FORMAT") == "true" {
		jsonData, err := json.MarshalIndent(u, "", "  ")
		if err != nil {
			return "Error formatting data"
		}
		return string(jsonData)
	}

	// For user display use beautiful formatting
	return u.FormatUserDataBeautifully()
}

// UserSession represents a user session
type UserSession struct {
	UserID          int64
	State           UserState
	Data            UserData
	CreatedAt       time.Time
	MessageCount    int       // Message counter
	LastCommandTime time.Time // Time of last command to avoid duplication
	LastCommand     string    // Last command to avoid duplication
	LastCallback    string    // Last callback to avoid duplication
	LastMessageID   int       // ID of last message with buttons
}

// NewUserSession creates a new user session
func NewUserSession(userID int64) *UserSession {
	return &UserSession{
		UserID:          userID,
		State:           StateInitial,
		Data:            UserData{},
		CreatedAt:       time.Now(),
		MessageCount:    0,
		LastCommandTime: time.Time{},
		LastCallback:    "",
		LastMessageID:   0,
	}
}

// IncrementMessageCount increases the message counter
func (s *UserSession) IncrementMessageCount() bool {
	const MaxMessages = 10 // Maximum number of messages
	s.MessageCount++
	return s.MessageCount <= MaxMessages
}

// CheckDuplicateCommand checks if a command is a duplicate
func (s *UserSession) CheckDuplicateCommand(command string) bool {
	// Check if this is a repeated command within 2 seconds
	if command != "" && command == s.LastCommand && time.Since(s.LastCommandTime) < 2*time.Second {
		return true
	}
	s.LastCommand = command
	s.LastCommandTime = time.Now()
	return false
}

// CheckDuplicateCallback checks if a callback is a duplicate
func (s *UserSession) CheckDuplicateCallback(callback string) bool {
	// Check if this is a repeated callback within 2 seconds
	if callback != "" && callback == s.LastCallback && time.Since(s.LastCommandTime) < 2*time.Second {
		return true
	}
	s.LastCallback = callback
	s.LastCommandTime = time.Now()
	return false
}

// GetKeyboardForState returns a keyboard depending on the current state
func (s *UserSession) GetKeyboardForState() *tgbotapi.InlineKeyboardMarkup {
	switch s.State {
	case StateAskSex:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Male", CallbackSex+"male"),
				tgbotapi.NewInlineKeyboardButtonData("Female", CallbackSex+"female"),
			),
		)
		return &keyboard

	case StateAskDiabetes:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Yes", CallbackDiabetes+"yes"),
				tgbotapi.NewInlineKeyboardButtonData("No", CallbackDiabetes+"no"),
			),
		)
		return &keyboard

	case StateAskLevel:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Beginner", CallbackLevel+"beginner"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Intermediate", CallbackLevel+"intermediate"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Advanced", CallbackLevel+"advanced"),
			),
		)
		return &keyboard

	case StateAskGoal:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Weight Loss", CallbackGoal+"weight_loss"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Muscle Gain", CallbackGoal+"muscle_gain"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Maintenance", CallbackGoal+"maintenance"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Endurance Improvement", CallbackGoal+"endurance"),
			),
		)
		return &keyboard

	case StateAskType:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Strength", CallbackType+"strength"),
				tgbotapi.NewInlineKeyboardButtonData("Cardio", CallbackType+"cardio"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Mixed", CallbackType+"mixed"),
				tgbotapi.NewInlineKeyboardButtonData("Yoga", CallbackType+"yoga"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Pilates", CallbackType+"pilates"),
				tgbotapi.NewInlineKeyboardButtonData("Other", CallbackType+"other"),
			),
		)
		return &keyboard

	case StatePayment:
		// Create payment URL in advance
		paymentURL, err := CreatePayment(s.UserID)
		if err != nil {
			log.Printf("Error creating payment link: %v", err)
			// If failed to create link, use callback button
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("💳 Pay", "pay"),
				),
			)
			return &keyboard
		}

		// Use URL button with nice emoji and more noticeable text
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("💳 Go to Payment", paymentURL),
			),
		)
		return &keyboard

	case StateComplete:
		// Buttons for questions after payment
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Clarify Nutrition", CallbackAsk+"nutrition"),
				tgbotapi.NewInlineKeyboardButtonData("Clarify Exercises", CallbackAsk+"exercises"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("How to Track Progress", CallbackAsk+"progress"),
				tgbotapi.NewInlineKeyboardButtonData("What to do with Diabetes", CallbackAsk+"diabetes"),
			),
		)
		return &keyboard
	}

	return nil
}

// GetNextQuestion returns the next question based on current state
func (s *UserSession) GetNextQuestion() string {
	switch s.State {
	case StateAskSex:
		return "Specify your gender:"
	case StateAskAge:
		return "Specify your age (full years):"
	case StateAskHeight:
		return "Specify your height in centimeters (for example, 175):"
	case StateAskWeight:
		return "Specify your weight in kilograms (for example, 70):"
	case StateAskDiabetes:
		return "Do you have diabetes?"
	case StateAskLevel:
		return "Rate your current fitness level:"
	case StateAskGoal:
		return "What is your main goal?"
	case StateAskType:
		return "What type of workouts do you prefer?"
	case StatePayment:
		return fmt.Sprintf("Thank you! Your information has been collected:\n\n%s\n\nTo receive a personalized workout program, please pay for the service. Click the button or enter /pay", s.Data.String())
	case StateComplete:
		return "Your personalized workout program has already been created. If you want to start over, use the /start command"
	default:
		return "Something went wrong. Try starting over with the /start command"
	}
}

// GetAskQuestionAnswer returns an answer to a question about the program
func (s *UserSession) GetAskQuestionAnswer(question string) string {
	switch question {
	case "nutrition":
		// Nutrition recommendations
		baseText := "🍽️ **NUTRITION RECOMMENDATIONS**\n\n"

		weight := s.Data.Weight
		height := s.Data.Height
		goal := s.Data.FitnessGoal

		if goal == "weight loss" {
			baseText += fmt.Sprintf("To achieve your weight loss goal, considering your weight %d kg and height %d cm, it is recommended to consume approximately %d-%d calories per day, with a deficit of 400-500 calories.\n\n",
				weight, height, (weight*30)-500, (weight*30)-400)
		} else if goal == "muscle gain" {
			baseText += fmt.Sprintf("For muscle mass gain, considering your weight %d kg, it is recommended to consume approximately %d-%d calories per day, with a surplus of 300-400 calories.\n\n",
				weight, (weight*30)+300, (weight*30)+400)
		} else {
			baseText += fmt.Sprintf("To maintain your current weight of %d kg, it is recommended to consume approximately %d-%d calories per day.\n\n",
				weight, weight*28, weight*30)
		}

		baseText += "Recommended macronutrient distribution:\n" +
			"- Protein: 1.6-2.0 g per kg of body weight (approximately " + fmt.Sprintf("%d-%d", int(float64(weight)*1.6), int(float64(weight)*2.0)) + " g per day)\n" +
			"- Fats: 0.8-1.0 g per kg of body weight (approximately " + fmt.Sprintf("%d-%d", int(float64(weight)*0.8), int(float64(weight)*1.0)) + " g per day)\n" +
			"- Carbohydrates: the remaining calories\n\n"

		baseText += "**Recommended meal schedule:**\n" +
			"1. Breakfast: protein food + complex carbohydrates (oatmeal, eggs, low-fat cottage cheese)\n" +
			"2. Snack: fruit or protein shake\n" +
			"3. Lunch: protein + vegetables + complex carbohydrates (meat/fish, vegetables, buckwheat/rice/quinoa)\n" +
			"4. Snack: nuts, yogurt, or cottage cheese\n" +
			"5. Dinner (at least 2-3 hours before sleep): protein + vegetables (chicken breast/fish, vegetable salad)\n\n"

		baseText += "**Recommendations for fluid intake:**\n" +
			fmt.Sprintf("- Drink at least %d ml of water per day\n", weight*30) +
			"- Drink a glass of water 30 minutes before each meal\n" +
			"- Limit alcohol and sweet drinks consumption\n\n"

		if s.Data.Diabetes == "yes" {
			baseText += "**Special recommendations for diabetes:**\n" +
				"- Avoid foods with high glycemic index\n" +
				"- Control carbohydrate portions\n" +
				"- Distribute carbohydrates evenly throughout the day\n" +
				"- Regularly measure blood sugar levels\n" +
				"- Consult with an endocrinologist for a detailed meal plan\n"
		}

		return baseText

	case "exercises":
		// Exercise recommendations
		baseText := "💪 **EXERCISE PROGRAM**\n\n"
		fitnessType := s.Data.FitnessType

		if fitnessType == "strength" {
			baseText += "**Strength Workout A (Monday):**\n" +
				"1. Warm-up: 5-10 minutes cardio and dynamic stretching\n" +
				"2. Squats: 4 sets of 10-12 repetitions\n" +
				"3. Bench press: 4 sets of 8-10 repetitions\n" +
				"4. Bent-over rows: 3 sets of 10-12 repetitions\n" +
				"5. Push-ups: 3 sets to failure\n" +
				"6. Plank: 3 sets of 30-60 seconds\n" +
				"7. Stretching: 5-10 minutes\n\n"

			baseText += "**Strength Workout B (Thursday):**\n" +
				"1. Warm-up: 5-10 minutes cardio and dynamic stretching\n" +
				"2. Deadlift: 4 sets of 8-10 repetitions\n" +
				"3. Overhead dumbbell press: 3 sets of 10-12 repetitions\n" +
				"4. Pull-ups (or lat pulldown): 3 sets to failure\n" +
				"5. Bicep curls: 3 sets of 12-15 repetitions\n" +
				"6. Tricep extensions: 3 sets of 12-15 repetitions\n" +
				"7. Stretching: 5-10 minutes\n\n"
		} else if fitnessType == "cardio" {
			baseText += "**Cardio Workout (Tuesday, Friday):**\n" +
				"1. Warm-up: 5 minutes of light walking or slow jogging\n" +
				"2. Interval training: 30 seconds sprint + 90 seconds walking (repeat 10 times)\n" +
				"3. Cool-down: 5 minutes slow walking\n\n"

			baseText += "**HIIT Workout (Saturday):**\n" +
				"1. Warm-up: 5 minutes\n" +
				"2. Circuit training (no rest between exercises, 60 sec rest between rounds):\n" +
				"   - Burpees: 30 seconds\n" +
				"   - Jump squats: 30 seconds\n" +
				"   - Mountain climbers: 30 seconds\n" +
				"   - Crunches: 30 seconds\n" +
				"   - Jump rope: 60 seconds\n" +
				"3. Repeat circuit 3-5 times\n" +
				"4. Cool-down and stretching: 5-10 minutes\n\n"
		} else {
			baseText += "**Full Body Workout (3 times a week - Mon, Wed, Fri):**\n" +
				"1. Warm-up: 5-10 minutes cardio and dynamic stretching\n" +
				"2. Squats: 3 sets of 12-15 repetitions\n" +
				"3. Push-ups: 3 sets of 10-12 repetitions\n" +
				"4. Back extensions: 3 sets of 12-15 repetitions\n" +
				"5. Plank: 3 sets of 30-60 seconds\n" +
				"6. Cardio: 15-20 minutes (running, cycling, elliptical)\n" +
				"7. Stretching: 5-10 minutes\n\n"
		}

		baseText += "**General recommendations:**\n" +
			"- Always start with a warm-up to avoid injuries\n" +
			"- Control proper exercise technique\n" +
			"- Gradually increase intensity every 2-3 weeks\n" +
			"- If you feel pain (not to be confused with muscle fatigue), stop the exercise\n" +
			"- Take 1-2 rest days per week for recovery\n\n"

		if s.Data.Level == "beginner" {
			baseText += "**Recommendations for beginners:**\n" +
				"- Start with lower weight and fewer repetitions\n" +
				"- Focus on learning proper technique\n" +
				"- Increase intensity gradually\n"
		}

		return baseText

	case "progress":
		// Progress tracking recommendations
		return "📊 **HOW TO TRACK PROGRESS**\n\n" +
			"**Main metrics to track:**\n" +
			"1. **Weight** - weigh yourself 1-2 times a week, at the same time (preferably in the morning on an empty stomach)\n" +
			"2. **Body measurements** - measure main body parts every 2-4 weeks:\n" +
			"   - Neck circumference\n" +
			"   - Chest circumference\n" +
			"   - Waist circumference\n" +
			"   - Hip circumference\n" +
			"   - Bicep circumference\n" +
			"   - Thigh circumference\n" +
			"   - Calf circumference\n" +
			"3. **Photos** - take photos in the same conditions (lighting, pose, clothing) every 4 weeks\n" +
			"4. **Workout journal** - record weights and repetitions for each exercise\n" +
			"5. **Food journal** - track calories and macronutrients consumed\n\n" +
			"**Additional parameters:**\n" +
			"- **Energy and well-being** - rate on a scale from 1 to 10\n" +
			"- **Sleep quality** - duration and feeling of rest after sleep\n" +
			"- **Workout performance** - how easy/difficult it is to perform exercises\n\n" +
			"**Technologies for tracking:**\n" +
			"- Calorie counting apps (MyFitnessPal, FatSecret)\n" +
			"- Workout apps (Strong, Jefit, Nike Training Club)\n" +
			"- Fitness trackers and smart watches for activity tracking\n\n" +
			"**How to evaluate results:**\n" +
			"- For weight loss: expect 0.5-1 kg loss per week (safe rate)\n" +
			"- For mass gain: 0.2-0.5 kg per week can be considered a good result\n" +
			"- Pay attention to changes in body size and well-being\n" +
			"- If progress stops for 2-3 weeks, review your program and nutrition\n\n" +
			"**Important to remember:**\n" +
			"- Progress is rarely linear\n" +
			"- Weight is affected by many factors (water, salt, hormones, stress)\n" +
			"- Evaluate progress comprehensively, not just by weight\n" +
			"- Be patient - sustainable results take time"

	case "diabetes":
		// Diabetes recommendations
		diabetesText := "🩺 **WORKOUTS WITH DIABETES**\n\n"

		if s.Data.Diabetes == "yes" {
			diabetesText += "**Main recommendations for workouts with diabetes:**\n\n" +
				"**Before workout:**\n" +
				"- Measure blood glucose level before workout\n" +
				"- If level is below 5.6 mmol/L, eat a small portion of carbohydrates\n" +
				"- If level is above 13.9 mmol/L, postpone intensive workout\n" +
				"- Have fast carbs with you (glucose, juice) in case of hypoglycemia\n" +
				"- Wear a diabetic identification bracelet\n\n" +
				"**During workout:**\n" +
				"- Pay attention to hypoglycemia symptoms (trembling, weakness, dizziness, sweating)\n" +
				"- For long workouts (more than 45-60 minutes) periodically check sugar level\n" +
				"- Drink enough water\n\n" +
				"**After workout:**\n" +
				"- Measure glucose level after workout\n" +
				"- Be attentive to delayed hypoglycemia (may occur 4-48 hours later)\n" +
				"- Make sure you have a post-workout meal plan\n\n" +
				"**Workout type selection:**\n" +
				"- Combine cardio and strength workouts for better sugar control\n" +
				"- Start with low intensity and gradually increase load\n" +
				"- Strength training improves insulin sensitivity\n" +
				"- Moderate intensity aerobic workouts (walking, swimming, cycling) are good for sugar control\n\n" +
				"**Insulin and medication dose adjustment:**\n" +
				"- Consult with your endocrinologist regarding insulin or medication dose adjustments before workout\n" +
				"- Usually requires reducing insulin dose before physical activity\n\n" +
				"**Important:**\n" +
				"- Always consult with your doctor before starting a new workout program\n" +
				"- Keep a journal, noting sugar levels before, during, and after workouts\n" +
				"- Be especially careful when working out in heat or cold\n" +
				"- Watch your feet condition and use appropriate footwear\n"
		} else {
			diabetesText += "You have not indicated diabetes, but here are general recommendations that are useful for everyone to know:\n\n" +
				"1. Regular physical activity helps maintain normal blood sugar levels\n" +
				"2. Balanced nutrition with control of simple carbohydrate intake supports healthy metabolism\n" +
				"3. Pay attention to your feelings during workouts - weakness, dizziness, increased thirst may indicate sugar problems\n" +
				"4. Regular medical examination will help identify potential problems at an early stage\n\n" +
				"Type 2 diabetes prevention:\n" +
				"- Maintain a healthy weight\n" +
				"- Exercise regularly\n" +
				"- Eat healthy food rich in fiber and low on glycemic index\n" +
				"- Limit sugar and refined carbohydrate intake\n" +
				"- Avoid smoking and excessive alcohol consumption\n"
		}

		return diabetesText

	default:
		return "Please clarify your question about the workout program."
	}
}

// ProcessButtonCallback processes button clicks
func (s *UserSession) ProcessButtonCallback(data string) (string, error) {
	if len(data) < 4 {
		return "Invalid data", fmt.Errorf("invalid callback data: %s", data)
	}

	if data == "pay" {
		// Create payment link
		paymentURL, err := CreatePayment(s.UserID)
		if err != nil {
			return "An error occurred while creating payment. Please try again later.", err
		}
		// Return link directly, bot will send it as a message
		return fmt.Sprintf("To make a payment, follow this link: %s", paymentURL), nil
	}

	// Process questions about workout program
	if strings.HasPrefix(data, "ask_") {
		question := strings.TrimPrefix(data, "ask_")
		return s.GetAskQuestionAnswer(question), nil
	}

	var prefix, value string

	// Determine prefix and value
	if strings.HasPrefix(data, CallbackSex) {
		prefix = CallbackSex
		value = data[len(CallbackSex):]
	} else if strings.HasPrefix(data, CallbackDiabetes) {
		prefix = CallbackDiabetes
		value = data[len(CallbackDiabetes):]
	} else if strings.HasPrefix(data, CallbackLevel) {
		prefix = CallbackLevel
		value = data[len(CallbackLevel):]
	} else if strings.HasPrefix(data, CallbackGoal) {
		prefix = CallbackGoal
		value = data[len(CallbackGoal):]
	} else if strings.HasPrefix(data, CallbackType) {
		prefix = CallbackType
		value = data[len(CallbackType):]
	} else if strings.HasPrefix(data, CallbackAsk) {
		prefix = CallbackAsk
		value = data[len(CallbackAsk):]
		return s.GetAskQuestionAnswer(value), nil
	} else if data == "pay" {
		return "/pay", nil
	} else {
		return "Unknown command", fmt.Errorf("unknown prefix in callback: %s", data)
	}

	// Process based on prefix
	switch prefix {
	case CallbackSex:
		s.Data.Sex = map[string]string{
			"male":   "male",
			"female": "female",
		}[value]
		s.State = StateAskAge

	case CallbackDiabetes:
		s.Data.Diabetes = map[string]string{
			"yes": "yes",
			"no":  "no",
		}[value]
		s.State = StateAskLevel

	case CallbackLevel:
		s.Data.Level = map[string]string{
			"beginner":     "beginner",
			"intermediate": "intermediate",
			"advanced":     "advanced",
		}[value]
		s.State = StateAskGoal

	case CallbackGoal:
		s.Data.FitnessGoal = map[string]string{
			"weight_loss": "weight loss",
			"muscle_gain": "muscle gain",
			"maintenance": "maintenance",
			"endurance":   "endurance improvement",
		}[value]
		s.State = StateAskType

	case CallbackType:
		s.Data.FitnessType = map[string]string{
			"strength": "strength",
			"cardio":   "cardio",
			"mixed":    "mixed",
			"yoga":     "yoga",
			"pilates":  "pilates",
			"other":    "other",
		}[value]
		s.State = StatePayment
	}

	// Return next question
	return s.GetNextQuestion(), nil
}

// FormatUserDataBeautifully returns beautifully formatted user data
func (u *UserData) FormatUserDataBeautifully() string {
	// Format readable representation of user data
	return fmt.Sprintf(
		"👤 *Your data*\n\n"+
			"• Gender: %s\n"+
			"• Age: %d years\n"+
			"• Height: %d cm\n"+
			"• Weight: %d kg\n"+
			"• Diabetes: %s\n"+
			"• Fitness level: %s\n"+
			"• Goal: %s\n"+
			"• Preferred workout type: %s",
		u.Sex, u.Age, u.Height, u.Weight, u.Diabetes,
		u.Level, u.FitnessGoal, u.FitnessType,
	)
}

// ProcessInput processes user input based on current state
func (s *UserSession) ProcessInput(input string) (string, error) {
	switch s.State {
	case StateInitial:
		s.State = StateAskSex
		return "Let's create a personalized fitness program for you! First, I'll ask you a few questions.\n\nSpecify your gender:", nil

	case StateAskSex:
		// If input by text, not buttons
		s.Data.Sex = input
		s.State = StateAskAge
		return "Specify your age (full years):", nil

	case StateAskAge:
		age, err := strconv.Atoi(input)
		if err != nil {
			return "Please enter age in digits (for example, 25):", nil
		}
		s.Data.Age = age
		s.State = StateAskHeight
		return "Specify your height in centimeters (for example, 175):", nil

	case StateAskHeight:
		height, err := strconv.Atoi(input)
		if err != nil {
			return "Please enter height in digits in centimeters (for example, 175):", nil
		}
		s.Data.Height = height
		s.State = StateAskWeight
		return "Specify your weight in kilograms (for example, 70):", nil

	case StateAskWeight:
		weight, err := strconv.Atoi(input)
		if err != nil {
			return "Please enter weight in digits in kilograms (for example, 70):", nil
		}
		s.Data.Weight = weight
		s.State = StateAskDiabetes
		return "Do you have diabetes?", nil

	case StateAskDiabetes:
		// If input by text, not buttons
		s.Data.Diabetes = input
		s.State = StateAskLevel
		return "Rate your current fitness level:", nil

	case StateAskLevel:
		// If input by text, not buttons
		s.Data.Level = input
		s.State = StateAskGoal
		return "What is your main goal?", nil

	case StateAskGoal:
		// If input by text, not buttons
		s.Data.FitnessGoal = input
		s.State = StateAskType
		return "What type of workouts do you prefer?", nil

	case StateAskType:
		// If input by text, not buttons
		s.Data.FitnessType = input
		s.State = StatePayment
		return fmt.Sprintf("Thank you! Your information has been collected:\n\n%s\n\nTo receive a personalized workout program, please pay for the service. Enter /pay", s.Data.String()), nil

	case StatePayment:
		if input == "/pay" {
			// If user entered /pay command, create link and send it in text
			paymentLink, err := CreatePayment(s.UserID)
			if err != nil {
				return "An error occurred while creating payment. Please try again later.", err
			}
			return fmt.Sprintf("To make a payment, follow this link: %s", paymentLink), nil
		}

		// Use beautiful data formatting instead of JSON
		return fmt.Sprintf("Thank you! Your information has been collected:\n\n%s\n\nTo receive a personalized workout program, please pay for the service. Click the button or enter /pay",
			s.Data.FormatUserDataBeautifully()), nil

	case StateComplete:
		return "Your personalized workout program has already been created. If you want to start over, use the /start command", nil

	default:
		return "Something went wrong. Try starting over with the /start command", nil
	}
}

// SetPaymentCompleted sets payment status as completed
func (s *UserSession) SetPaymentCompleted(paymentID string) {
	s.Data.PaymentID = paymentID
	s.State = StateComplete
	s.Data.CreatedAt = time.Now()
}
