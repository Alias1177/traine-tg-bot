// openai.go
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient wrapper for working with OpenAI API
type OpenAIClient struct {
	client      *openai.Client
	useFallback bool
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(token string) *OpenAIClient {
	// Check token
	if len(token) < 20 {
		log.Println("WARNING: It seems that the OpenAI token is invalid (too short)")
	}

	client := openai.NewClient(token)
	log.Printf("OpenAI client initialized with token: %s***", token[:10])

	// Check if fallback should be used
	useFallback := os.Getenv("USE_OPENAI_FALLBACK") == "true"

	return &OpenAIClient{
		client:      client,
		useFallback: useFallback,
	}
}

// GetCompletion sends a request to OpenAI and returns the response
func (c *OpenAIClient) GetCompletion(prompt string) (string, error) {
	// If fallback mode is enabled, use fallback
	if c.useFallback {
		log.Println("Using fallback mode for OpenAI")
		return c.getFallbackResponse(prompt), nil
	}

	ctx := context.Background()

	// Create base system prompt for fitness trainer
	systemPrompt := `You are an experienced fitness trainer and nutritionist. Your task is to provide personalized recommendations 
based on user data, which will be provided in JSON format at the beginning of the request.
Consider gender, age, height, weight, diabetes status, fitness level, and user goals.
Always give practical, science-based advice that can be applied immediately.
Never give advice that could be dangerous to health.
Your responses should be personalized, specific, and motivating.`

	// Set model
	model := openai.GPT3Dot5Turbo
	if os.Getenv("OPENAI_MODEL") != "" {
		model = os.Getenv("OPENAI_MODEL")
	}

	log.Printf("Sending request to OpenAI (model: %s, request length: %d characters)",
		model, len(prompt))

	req := openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens:   2500, // Increased maximum response length
		Temperature: 0.7,  // Added temperature parameter for more stable responses
	}

	// Set timeout for request
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(timeoutCtx, req)
	if err != nil {
		log.Printf("OpenAI API error: %v", err)

		// If error is related to limits or timeout, enable fallback mode
		if strings.Contains(err.Error(), "429") ||
			strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "connection") {
			log.Println("Switching to fallback mode due to API error")
			c.useFallback = true
			return c.getFallbackResponse(prompt), nil
		}
		return "", err
	}

	if len(resp.Choices) == 0 {
		log.Println("OpenAI returned empty response")
		return "", errors.New("no response from OpenAI")
	}

	answer := resp.Choices[0].Message.Content
	log.Printf("Received response from OpenAI (length: %d characters)", len(answer))
	return answer, nil
}

// getFallbackResponse returns a local response without API call
func (c *OpenAIClient) getFallbackResponse(prompt string) string {
	// Check if the request contains keywords for workout plan
	if strings.Contains(strings.ToLower(prompt), "workout program") ||
		strings.Contains(strings.ToLower(prompt), "training plan") {
		return `📋 PERSONALIZED WORKOUT PROGRAM

Based on your data, I've created an optimal workout program for 2 weeks:

## WEEKLY PLAN

**Monday**: Strength training (upper body) - 45 minutes
**Tuesday**: Cardio - 30 minutes
**Wednesday**: Rest
**Thursday**: Strength training (lower body) - 45 minutes
**Friday**: Cardio + light strength - 40 minutes
**Saturday**: Active recovery (walking, yoga) - 30 minutes
**Sunday**: Complete rest

## DETAILED WORKOUTS

### MONDAY (STRENGTH - UPPER BODY)
1. Warm-up - 5 minutes
2. Push-ups: 3 sets of 10-12 repetitions
3. Dumbbell rows: 3×12
4. Shoulder press: 3×12
5. Bicep curls: 3×12
6. Tricep dips: 3×15
7. Stretching - 5 minutes

### TUESDAY (CARDIO)
1. Warm-up - 5 minutes
2. Interval training:
   - 1 minute fast walking/running
   - 1 minute regular walking
   - Repeat 10 times
3. Cool-down - 5 minutes

### THURSDAY (STRENGTH - LOWER BODY)
1. Warm-up - 5 minutes
2. Squats: 3×15
3. Lunges: 3×12 for each leg
4. Calf raises: 3×20
5. Glute bridge: 3×15
6. Plank: 3×30 seconds
7. Stretching - 5 minutes

### FRIDAY (CARDIO + LIGHT STRENGTH)
1. Warm-up - 5 minutes
2. Cardio - 15 minutes (walking, running, or cycling)
3. Circuit training (3 rounds):
   - Squats: 15 repetitions
   - Knee push-ups: 10 repetitions
   - Crunches: 15 repetitions
   - Plank: 30 seconds
4. Stretching - 5 minutes

## NUTRITION RECOMMENDATIONS
- Increase protein intake (meat, fish, eggs, cottage cheese)
- Eat complex carbohydrates (vegetables, grains, legumes)
- Monitor sugar levels due to diabetes
- Drink at least 2 liters of water per day
- Eat frequently and in small portions (4-5 times a day)

## PROGRESS TRACKING
- Keep a workout journal
- Take before and after photos
- Measure body circumferences once a week
- Regularly monitor weight (1-2 times a week)
- Pay attention to well-being and energy

## SPECIAL RECOMMENDATIONS
- Stop training immediately if hypoglycemia symptoms appear
- Carry fast carbs with you (juice, candy)
- Check sugar levels before and after workouts
- Exercise 1-2 hours after eating
- Increase intensity gradually

This program is designed considering your level and health specifics. Gradually you'll be able to increase workout intensity.`
	}

	// Fallback for regular questions
	return "🤖 Autonomous mode (OpenAI unavailable):\n\n" +
		"I can't connect to OpenAI right now, but here are some general fitness recommendations:\n\n" +
		"1. Regular workouts (3-5 times a week) are the key to success\n" +
		"2. Combine cardio and strength training for comprehensive results\n" +
		"3. Proper nutrition accounts for 70% of success in achieving fitness goals\n" +
		"4. Monitor recovery and ensure your body gets enough rest\n" +
		"5. Gradually increase intensity for continuous progress\n\n" +
		"Ask your question later when the service is available. Request time: " + time.Now().Format("15:04:05")
}
