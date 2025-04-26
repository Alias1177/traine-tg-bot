// payment.go
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
)

// InitStripe инициализирует Stripe API
func InitStripe() {
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeKey == "" {
		log.Println("ВНИМАНИЕ: STRIPE_SECRET_KEY не установлен, используется тестовый ключ!")
		stripeKey = "sk_test_51OvzFvICWrE4bwxoFbvNn5ZLsrJHMfgFu0i12PeMmrgnxbXBVcxZmV3Oj8yN2OJauxfAyHhk2WbRSLGLYMZgOWBq00T3rdtUQY"
	}
	stripe.Key = stripeKey
	log.Printf("Stripe API инициализирован с ключом: %s***", stripeKey[:10])
}

// PaymentConfig содержит конфигурацию для платежей
type PaymentConfig struct {
	ProductName   string
	ProductDesc   string
	PriceAmount   int64 // в минимальных единицах валюты (центы, копейки и т.д.)
	Currency      string
	SuccessURL    string
	CancelURL     string
	WebhookSecret string
}

// GetDefaultPaymentConfig возвращает конфигурацию по умолчанию
func GetDefaultPaymentConfig() PaymentConfig {
	baseURL := os.Getenv("BOT_WEBHOOK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://example.com" // Заглушка, нужно заменить на реальный URL
	}

	return PaymentConfig{
		ProductName:   "Персональная фитнес-программа",
		ProductDesc:   "Индивидуальная программа тренировок, созданная с учетом ваших параметров и целей",
		PriceAmount:   5000, // 50.00 валютных единиц
		Currency:      "rub",
		SuccessURL:    fmt.Sprintf("%s/payment/success?session_id={CHECKOUT_SESSION_ID}", baseURL),
		CancelURL:     fmt.Sprintf("%s/payment/cancel?session_id={CHECKOUT_SESSION_ID}", baseURL),
		WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
}

// CreatePayment создает платежную сессию Stripe и возвращает URL для оплаты
func CreatePayment(userID int64) (string, error) {
	config := GetDefaultPaymentConfig()

	// Проверяем минимальный размер платежа
	if config.PriceAmount < 5000 {
		log.Printf("ВНИМАНИЕ: Сумма платежа %d может быть слишком маленькой для Stripe", config.PriceAmount)
	}

	// Преобразуем ID пользователя в строку и логируем его
	userIDStr := strconv.FormatInt(userID, 10)
	log.Printf("Создание платежа для пользователя ID: %s", userIDStr)

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(config.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(config.ProductName),
						Description: stripe.String(config.ProductDesc),
					},
					UnitAmount: stripe.Int64(config.PriceAmount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:        stripe.String(config.SuccessURL),
		CancelURL:         stripe.String(config.CancelURL),
		ClientReferenceID: stripe.String(userIDStr),
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("Ошибка создания сессии Stripe: %v", err)
		return "", err
	}

	log.Printf("Создана сессия Stripe %s для пользователя %s с URL: %s", s.ID, userIDStr, s.URL)
	return s.URL, nil
}

// VerifyPayment проверяет статус оплаты
func VerifyPayment(sessionID string) (bool, string, error) {
	log.Printf("Проверка статуса платежа для сессии: %s", sessionID)

	// Дополнительная проверка параметров
	if sessionID == "" {
		return false, "", fmt.Errorf("пустой ID сессии")
	}

	s, err := session.Get(sessionID, nil)
	if err != nil {
		log.Printf("Ошибка получения данных сессии %s: %v", sessionID, err)
		return false, "", err
	}

	// Выводим все данные о сессии для отладки
	log.Printf("Сессия: %s, Статус платежа: %s, ID клиента: %s, Режим: %s",
		s.ID, s.PaymentStatus, s.ClientReferenceID, s.Mode)

	// Для локального тестирования - всегда считаем платеж успешным
	if os.Getenv("STRIPE_TEST_MODE") == "true" {
		log.Printf("ТЕСТОВЫЙ РЕЖИМ: Считаем платеж успешным")
		return true, s.ClientReferenceID, nil
	}

	// Проверяем статус платежа
	if s.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid {
		log.Printf("Платеж подтвержден для сессии: %s", sessionID)
		return true, s.ClientReferenceID, nil
	}

	log.Printf("Платеж не подтвержден для сессии: %s (статус: %s)", sessionID, s.PaymentStatus)
	return false, s.ClientReferenceID, nil
}

// ManuallyCompletePayment позволяет вручную завершить платеж для тестирования
func ManuallyCompletePayment(userID int64) string {
	// Генерируем фиктивный ID сессии
	sessionID := fmt.Sprintf("cs_test_manual_%d_%d", userID, time.Now().Unix())
	log.Printf("Вручную завершен платеж: %s для пользователя %d", sessionID, userID)
	return sessionID
}
