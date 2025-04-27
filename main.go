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

// WebhookHandler обрабатывает webhook-события
type WebhookHandler struct {
	bot           *Bot
	webhookSecret string
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("Получен запрос %s %s", r.Method, r.URL.Path)

	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Ошибка чтения webhook: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Выводим полезную информацию для отладки
	log.Printf("Получен webhook, длина: %d байт, заголовки: %v", len(payload), r.Header)

	// Проверяем подпись webhook если секрет установлен
	var event stripe.Event
	if h.webhookSecret != "" {
		event, err = webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.webhookSecret)
		if err != nil {
			log.Printf("Ошибка проверки подписи webhook: %v", err)
			// Если в локальном режиме, логируем полученный payload
			if os.Getenv("LOG_WEBHOOK_PAYLOAD") == "true" {
				log.Printf("Полученный webhook payload: %s", string(payload))
			}

			// Если в тестовом режиме, продолжаем несмотря на ошибку подписи
			if !strings.Contains(err.Error(), "signature") || os.Getenv("STRIPE_TEST_MODE") != "true" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Пытаемся распарсить событие без проверки подписи (для тестирования)
			if err := json.Unmarshal(payload, &event); err != nil {
				log.Printf("Ошибка разбора события без подписи: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			log.Printf("ВНИМАНИЕ: Обработка события без проверки подписи (только для тестирования)")
		}
	} else {
		// Если секрет не установлен, просто разбираем JSON
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("Ошибка разбора события: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Printf("ВНИМАНИЕ: Webhook Secret не установлен, подпись не проверяется")
	}

	// Логирование полученного события
	log.Printf("Получено событие Stripe: %s [%s]", event.Type, event.ID)

	// Обрабатываем событие
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("Ошибка разбора события checkout.session.completed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("Обработка успешной оплаты: %s, для пользователя: %s", session.ID, session.ClientReferenceID)

		err = h.bot.ProcessPaymentWebhook(session.ID)
		if err != nil {
			log.Printf("Ошибка обработки платежа: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		log.Printf("Успешно обработан платеж: %s", session.ID)
	case "payment_intent.succeeded":
		log.Printf("Получено событие payment_intent.succeeded, но обработка происходит по checkout.session.completed")
	default:
		log.Printf("Получено необрабатываемое событие типа: %s", event.Type)
	}

	elapsed := time.Since(start)
	log.Printf("Обработка webhook заняла %s", elapsed)
	w.WriteHeader(http.StatusOK)
}

func main() {
	// Настраиваем логирование
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Запуск приложения")

	// Загрузка конфигурации
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Инициализация OpenAI клиента
	openAIClient := NewOpenAIClient(config.OpenAIToken)

	// Инициализация и запуск бота
	bot, err := NewBot(config.TelegramToken, openAIClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}

	// Запуск обработки сообщений в отдельной горутине
	go bot.Start()
	fmt.Println("Бот запущен...")

	// Настройка HTTP сервера для webhook'ов
	webhookHandler := &WebhookHandler{
		bot:           bot,
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	// Выводим дополнительную информацию для отладки
	log.Printf("Настройка webhook для Stripe на пути /webhook/stripe")
	http.Handle("/webhook/stripe", webhookHandler)
	http.Handle("/webhook", webhookHandler) // Альтернативный путь для webhook

	// Для отладки - простой handler, чтобы проверить работу сервера
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Получен ping-запрос")
		w.Write([]byte("pong"))
	})

	// Мониторинг
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Запрос статуса сервера")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})
	// Настройка статических файлов
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Обработчик успешной оплаты
	http.HandleFunc("/payment/success", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Получен запрос на страницу успешной оплаты: %s", r.URL.String())

		// Получаем ID сессии из URL
		sessionID := r.URL.Query().Get("session_id")
		if sessionID != "" {
			log.Printf("ID сессии оплаты: %s", sessionID)

			// Пытаемся обработать успешный платеж через webhook,
			// если пользователь вернулся через success_url
			if os.Getenv("STRIPE_TEST_MODE") == "true" {
				go func() {
					log.Printf("Тестовый режим: автоматическая обработка успешного платежа")
					// Даем время на обработку обычного webhook
					time.Sleep(2 * time.Second)

					// Обрабатываем платеж, если он еще не был обработан
					webhookHandler.bot.ProcessPaymentWebhook(sessionID)
				}()
			}
		}

		// Отправляем HTML-страницу успешной оплаты
		http.ServeFile(w, r, "./static/succed.html")
	})

	// Обработчик отмены оплаты
	http.HandleFunc("/payment/cancel", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Получен запрос на страницу отмены оплаты")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
        <html>
        <head>
            <meta charset="UTF-8">
            <title>Оплата отменена</title>
            <script>
                // Автоматический редирект в Telegram через 3 секунды
                window.onload = function() {
                    setTimeout(function() {
                        window.location.href = 'tg://';
                        
                        // Запасной вариант, если tg:// не сработает
                        setTimeout(function() {
                            window.location.href = 'https://web.telegram.org/';
                        }, 1000);
                    }, 3000);
                }
            </script>
        </head>
        <body style="text-align: center; margin-top: 50px;">
            <h1>Оплата отменена</h1>
            <p>Вы будете перенаправлены обратно в Telegram через 3 секунды...</p>
            <a href="tg://">Вернуться в Telegram сейчас</a>
        </body>
        </html>
    `))
	})

	// Запуск HTTP сервера
	port := os.Getenv("PORT")
	if port == "" {
		port = "4242"
	}

	go func() {
		log.Printf("Запуск HTTP сервера на порту %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Ошибка запуска HTTP сервера: %v", err)
		}
	}()

	// Настройка graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Завершение работы бота...")
}
