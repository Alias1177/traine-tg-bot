// main.go
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/webhook"
)

// WebhookHandler processes webhook events
type WebhookHandler struct {
	bot           *Bot
	webhookSecret string
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("Received request %s %s", r.Method, r.URL.Path)

	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading webhook: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Output useful information for debugging
	log.Printf("Received webhook, length: %d bytes, headers: %v", len(payload), r.Header)

	// Check webhook signature if secret is set
	var event stripe.Event
	if h.webhookSecret != "" {
		event, err = webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.webhookSecret)
		if err != nil {
			log.Printf("Error verifying webhook signature: %v", err)
			// If in local mode, log the received payload
			if os.Getenv("LOG_WEBHOOK_PAYLOAD") == "true" {
				log.Printf("Received webhook payload: %s", string(payload))
			}

			// If in test mode, continue despite signature error
			if !strings.Contains(err.Error(), "signature") || os.Getenv("STRIPE_TEST_MODE") != "true" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Try to parse the event without signature verification (for testing)
			if err := json.Unmarshal(payload, &event); err != nil {
				log.Printf("Error parsing event without signature: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			log.Printf("WARNING: Processing event without signature verification (for testing only)")
		}
	} else {
		// If secret is not set, just parse JSON
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("Error parsing event: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Printf("WARNING: Webhook Secret not set, signature not verified")
	}

	// Log received event
	log.Printf("Received Stripe event: %s [%s]", event.Type, event.ID)

	// Process event
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("Error parsing checkout.session.completed event: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("Processing successful payment: %s, for user: %s", session.ID, session.ClientReferenceID)

		err = h.bot.ProcessPaymentWebhook(session.ID)
		if err != nil {
			log.Printf("Error processing payment: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		log.Printf("Successfully processed payment: %s", session.ID)
	case "payment_intent.succeeded":
		log.Printf("Received payment_intent.succeeded event, but processing happens on checkout.session.completed")
	default:
		log.Printf("Received unprocessed event type: %s", event.Type)
	}

	elapsed := time.Since(start)
	log.Printf("Webhook processing took %s", elapsed)
	w.WriteHeader(http.StatusOK)
}

func main() {
	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Starting application")

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize OpenAI client
	openAIClient := NewOpenAIClient(config.OpenAIToken)

	// Initialize and start bot
	bot, err := NewBot(config.TelegramToken, openAIClient)
	if err != nil {
		log.Fatalf("Error initializing bot: %v", err)
	}

	// Start message processing in a separate goroutine
	go bot.Start()
	fmt.Println("Bot started...")

	// Configure HTTP server for webhooks
	webhookHandler := &WebhookHandler{
		bot:           bot,
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	// Output additional information for debugging
	log.Printf("Setting up webhook for Stripe at path /webhook/stripe")
	http.Handle("/webhook/stripe", webhookHandler)
	http.Handle("/webhook", webhookHandler) // Alternative path for webhook

	// For debugging - simple handler to check server operation
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received ping request")
		w.Write([]byte("pong"))
	})

	// Monitoring
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Server status request")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})
	// Setup static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Successful payment handler
	http.HandleFunc("/payment/success", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request to success payment page: %s", r.URL.String())

		// Get session ID from URL
		sessionID := r.URL.Query().Get("session_id")
		if sessionID != "" {
			log.Printf("Payment session ID: %s", sessionID)

			// Try to process successful payment through webhook,
			// if user returned via success_url
			if os.Getenv("STRIPE_TEST_MODE") == "true" {
				go func() {
					log.Printf("Test mode: automatic processing of successful payment")
					// Give time for normal webhook processing
					time.Sleep(2 * time.Second)

					// Process payment if it hasn't been processed yet
					webhookHandler.bot.ProcessPaymentWebhook(sessionID)
				}()
			}
		}

		// Send HTML success payment page
		http.ServeFile(w, r, "./static/succed.html")
	})

	// Payment cancellation handler
	http.HandleFunc("/payment/cancel", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request to cancel payment page")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
        <html>
        <head>
            <meta charset="UTF-8">
            <title>Payment Cancelled</title>
            <script>
                // Automatic redirect to Telegram after 3 seconds
                window.onload = function() {
                    setTimeout(function() {
                        window.location.href = 'tg://';
                        
                        // Fallback if tg:// doesn't work
                        setTimeout(function() {
                            window.location.href = 'https://web.telegram.org/';
                        }, 1000);
                    }, 3000);
                }
            </script>
        </head>
        <body style="text-align: center; margin-top: 50px;">
            <h1>Payment Cancelled</h1>
            <p>You will be redirected back to Telegram in 3 seconds...</p>
            <a href="tg://">Return to Telegram now</a>
        </body>
        </html>
    `))
	})

	// Start HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "4242"
	}

	go func() {
		log.Printf("Starting HTTP server on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Error starting HTTP server: %v", err)
		}
	}()

	// Configure graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down bot...")
}
