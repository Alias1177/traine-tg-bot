# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Копирование и загрузка зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копирование исходного кода
COPY . .

# Сборка приложения
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o telegram-gpt-bot .

# Финальный образ
FROM alpine:latest

WORKDIR /root/

# Установка CA сертификатов
RUN apk --no-cache add ca-certificates

# Копирование бинарного файла из образа builder
COPY --from=builder /app/telegram-gpt-bot .

# Запуск приложения
CMD ["./telegram-gpt-bot"]