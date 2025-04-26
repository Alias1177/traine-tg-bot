// user.go
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// UserState представляет состояние диалога с пользователем
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

// UserData структура для хранения данных пользователя
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

// String возвращает строковое представление данных пользователя
func (u *UserData) String() string {
	jsonData, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return "Ошибка форматирования данных"
	}
	return string(jsonData)
}

// UserSession представляет сессию пользователя
type UserSession struct {
	UserID    int64
	State     UserState
	Data      UserData
	CreatedAt time.Time
}

// NewUserSession создает новую сессию пользователя
func NewUserSession(userID int64) *UserSession {
	return &UserSession{
		UserID:    userID,
		State:     StateInitial,
		Data:      UserData{},
		CreatedAt: time.Now(),
	}
}

// ProcessInput обрабатывает ввод пользователя на основе текущего состояния
func (s *UserSession) ProcessInput(input string) (string, error) {
	switch s.State {
	case StateInitial:
		s.State = StateAskSex
		return "Давайте создадим для вас персональную фитнес-программу! Сначала я задам несколько вопросов.\n\nУкажите ваш пол (мужской/женский):", nil

	case StateAskSex:
		s.Data.Sex = input
		s.State = StateAskAge
		return "Укажите ваш возраст (полных лет):", nil

	case StateAskAge:
		age, err := strconv.Atoi(input)
		if err != nil {
			return "Пожалуйста, введите возраст цифрами (например, 25):", nil
		}
		s.Data.Age = age
		s.State = StateAskHeight
		return "Укажите ваш рост в сантиметрах (например, 175):", nil

	case StateAskHeight:
		height, err := strconv.Atoi(input)
		if err != nil {
			return "Пожалуйста, введите рост цифрами в сантиметрах (например, 175):", nil
		}
		s.Data.Height = height
		s.State = StateAskWeight
		return "Укажите ваш вес в килограммах (например, 70):", nil

	case StateAskWeight:
		weight, err := strconv.Atoi(input)
		if err != nil {
			return "Пожалуйста, введите вес цифрами в килограммах (например, 70):", nil
		}
		s.Data.Weight = weight
		s.State = StateAskDiabetes
		return "У вас есть диабет? (да/нет):", nil

	case StateAskDiabetes:
		s.Data.Diabetes = input
		s.State = StateAskLevel
		return "Оцените ваш текущий уровень физической подготовки (начинающий/средний/продвинутый):", nil

	case StateAskLevel:
		s.Data.Level = input
		s.State = StateAskGoal
		return "Какова ваша главная цель? (похудение/набор массы/поддержание формы/улучшение выносливости):", nil

	case StateAskGoal:
		s.Data.FitnessGoal = input
		s.State = StateAskType
		return "Какой тип тренировок вы предпочитаете? (силовые/кардио/смешанные/йога/пилатес/другое):", nil

	case StateAskType:
		s.Data.FitnessType = input
		s.State = StatePayment
		return fmt.Sprintf("Спасибо! Ваша информация собрана:\n\n%s\n\nДля получения персональной программы тренировок, пожалуйста, оплатите услугу. Нажмите /pay чтобы перейти к оплате.", s.Data.String()), nil

	case StatePayment:
		if input == "/pay" {
			// Здесь будет логика создания платежа
			paymentLink, err := CreatePayment(s.UserID)
			if err != nil {
				return "Произошла ошибка при создании платежа. Пожалуйста, попробуйте позже.", err
			}
			return fmt.Sprintf("Для оплаты перейдите по ссылке: %s", paymentLink), nil
		}
		return "Для оплаты введите команду /pay", nil

	case StateComplete:
		return "Ваша персональная программа тренировок уже создана. Если вы хотите начать заново, используйте команду /start", nil

	default:
		return "Что-то пошло не так. Попробуйте начать сначала с команды /start", nil
	}
}

// SetPaymentCompleted устанавливает статус оплаты как завершенный
func (s *UserSession) SetPaymentCompleted(paymentID string) {
	s.Data.PaymentID = paymentID
	s.State = StateComplete
	s.Data.CreatedAt = time.Now()
}
