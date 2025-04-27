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

// InitStripe initializes Stripe API
func InitStripe() {
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeKey == "" {
		log.Println("WARNING: STRIPE_SECRET_KEY not set, using test key!")
	}
	stripe.Key = stripeKey
	log.Printf("Stripe API initialized with key: %s***", stripeKey[:10])
}

// PaymentConfig contains payment configuration
type PaymentConfig struct {
	ProductName   string
	ProductDesc   string
	PriceAmount   int64 // in minimum currency units (cents, kopecks, etc.)
	Currency      string
	SuccessURL    string
	CancelURL     string
	WebhookSecret string
}

// GetDefaultPaymentConfig returns default configuration
func GetDefaultPaymentConfig() PaymentConfig {
	// Define base URL
	baseURL := os.Getenv("BOT_WEBHOOK_BASE_URL")
	if baseURL == "" {
		// For local testing
		port := os.Getenv("PORT")
		if port == "" {
			port = "4242"
		}
		baseURL = fmt.Sprintf("http://localhost:%s", port)

		// Check Stripe mode
		if os.Getenv("STRIPE_TEST_MODE") == "true" {
			log.Println("Working in Stripe test mode, redirect URLs will be ignored")
		} else {
			log.Println("WARNING: Set BOT_WEBHOOK_BASE_URL for proper redirect operation!")
		}
	}

	return PaymentConfig{
		ProductName:   "Personalized Fitness Program",
		ProductDesc:   "Individual workout program created based on your parameters and goals",
		PriceAmount:   5000, // 50.00 currency units
		Currency:      "rub",
		SuccessURL:    fmt.Sprintf("%s/payment/success?session_id={CHECKOUT_SESSION_ID}", baseURL),
		CancelURL:     fmt.Sprintf("%s/payment/cancel?session_id={CHECKOUT_SESSION_ID}", baseURL),
		WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
}

// CreatePayment creates a Stripe payment session and returns URL for payment
func CreatePayment(userID int64) (string, error) {
	config := GetDefaultPaymentConfig()

	// Check minimum payment amount
	if config.PriceAmount < 5000 {
		log.Printf("WARNING: Payment amount %d may be too small for Stripe", config.PriceAmount)
	}

	// Convert user ID to string and log it
	userIDStr := strconv.FormatInt(userID, 10)
	log.Printf("Creating payment for user ID: %s", userIDStr)

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
		log.Printf("Error creating Stripe session: %v", err)
		return "", err
	}

	log.Printf("Created Stripe session %s for user %s with URL: %s", s.ID, userIDStr, s.URL)
	return s.URL, nil
}

// VerifyPayment checks payment status
func VerifyPayment(sessionID string) (bool, string, error) {
	log.Printf("Checking payment status for session: %s", sessionID)

	// Additional parameter check
	if sessionID == "" {
		return false, "", fmt.Errorf("empty session ID")
	}

	s, err := session.Get(sessionID, nil)
	if err != nil {
		log.Printf("Error getting session data %s: %v", sessionID, err)
		return false, "", err
	}

	// Output all session data for debugging
	log.Printf("Session: %s, Payment status: %s, Client ID: %s, Mode: %s",
		s.ID, s.PaymentStatus, s.ClientReferenceID, s.Mode)

	// For local testing - always consider payment successful
	if os.Getenv("STRIPE_TEST_MODE") == "true" {
		log.Printf("TEST MODE: Considering payment successful")
		return true, s.ClientReferenceID, nil
	}

	// Check payment status
	if s.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid {
		log.Printf("Payment confirmed for session: %s", sessionID)
		return true, s.ClientReferenceID, nil
	}

	log.Printf("Payment not confirmed for session: %s (status: %s)", sessionID, s.PaymentStatus)
	return false, s.ClientReferenceID, nil
}

// ManuallyCompletePayment allows manually completing payment for testing
func ManuallyCompletePayment(userID int64) string {
	// Generate a fake session ID
	sessionID := fmt.Sprintf("cs_test_manual_%d_%d", userID, time.Now().Unix())
	log.Printf("Payment manually completed: %s for user %d", sessionID, userID)
	return sessionID
}
