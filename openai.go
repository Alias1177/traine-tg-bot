package main

import (
	"context"
	"errors"

	"github.com/sashabaranov/go-openai"
)

// OpenAIClient обертка для работы с OpenAI API
type OpenAIClient struct {
	client *openai.Client
}

// NewOpenAIClient создает нового клиента OpenAI
func NewOpenAIClient(token string) *OpenAIClient {
	client := openai.NewClient(token)
	return &OpenAIClient{
		client: client,
	}
}

// GetCompletion отправляет запрос к OpenAI и возвращает ответ
func (c *OpenAIClient) GetCompletion(prompt string) (string, error) {
	ctx := context.Background()

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens: 2000, // Максимальная длина ответа
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("нет ответа от OpenAI")
	}

	return resp.Choices[0].Message.Content, nil
}
