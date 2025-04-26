// user.go
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

// CallbackPrefix - префиксы для callback данных
const (
	CallbackSex      = "sex:"
	CallbackDiabetes = "dia:"
	CallbackLevel    = "lvl:"
	CallbackGoal     = "gol:"
	CallbackType     = "typ:"
	CallbackAsk      = "ask:"
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
	UserID          int64
	State           UserState
	Data            UserData
	CreatedAt       time.Time
	MessageCount    int       // Счетчик сообщений
	LastCommandTime time.Time // Время последней команды для избежания дублирования
	LastCommand     string    // Последняя команда для избежания дублирования
	LastCallback    string    // Последний callback для избежания дублирования
	LastMessageID   int       // ID последнего сообщения с кнопками
}

// NewUserSession создает новую сессию пользователя
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

// IncrementMessageCount увеличивает счетчик сообщений
func (s *UserSession) IncrementMessageCount() bool {
	const MaxMessages = 10 // Максимальное количество сообщений
	s.MessageCount++
	return s.MessageCount <= MaxMessages
}

// CheckDuplicateCommand проверяет, является ли команда дубликатом
func (s *UserSession) CheckDuplicateCommand(command string) bool {
	// Проверяем, не является ли это повторной командой в течение 2 секунд
	if command != "" && command == s.LastCommand && time.Since(s.LastCommandTime) < 2*time.Second {
		return true
	}
	s.LastCommand = command
	s.LastCommandTime = time.Now()
	return false
}

// CheckDuplicateCallback проверяет, является ли callback дубликатом
func (s *UserSession) CheckDuplicateCallback(callback string) bool {
	// Проверяем, не является ли это повторным callback в течение 2 секунд
	if callback != "" && callback == s.LastCallback && time.Since(s.LastCommandTime) < 2*time.Second {
		return true
	}
	s.LastCallback = callback
	s.LastCommandTime = time.Now()
	return false
}

// GetKeyboardForState возвращает клавиатуру в зависимости от текущего состояния
func (s *UserSession) GetKeyboardForState() *tgbotapi.InlineKeyboardMarkup {
	switch s.State {
	case StateAskSex:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Мужской", CallbackSex+"male"),
				tgbotapi.NewInlineKeyboardButtonData("Женский", CallbackSex+"female"),
			),
		)
		return &keyboard

	case StateAskDiabetes:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Да", CallbackDiabetes+"yes"),
				tgbotapi.NewInlineKeyboardButtonData("Нет", CallbackDiabetes+"no"),
			),
		)
		return &keyboard

	case StateAskLevel:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Начинающий", CallbackLevel+"beginner"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Средний", CallbackLevel+"intermediate"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Продвинутый", CallbackLevel+"advanced"),
			),
		)
		return &keyboard

	case StateAskGoal:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Похудение", CallbackGoal+"weight_loss"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Набор массы", CallbackGoal+"muscle_gain"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Поддержание формы", CallbackGoal+"maintenance"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Улучшение выносливости", CallbackGoal+"endurance"),
			),
		)
		return &keyboard

	case StateAskType:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Силовые", CallbackType+"strength"),
				tgbotapi.NewInlineKeyboardButtonData("Кардио", CallbackType+"cardio"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Смешанные", CallbackType+"mixed"),
				tgbotapi.NewInlineKeyboardButtonData("Йога", CallbackType+"yoga"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Пилатес", CallbackType+"pilates"),
				tgbotapi.NewInlineKeyboardButtonData("Другое", CallbackType+"other"),
			),
		)
		return &keyboard

	case StatePayment:
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Оплатить", "pay"),
			),
		)
		return &keyboard

	case StateComplete:
		// Кнопки для вопросов после оплаты
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Уточнить питание", CallbackAsk+"nutrition"),
				tgbotapi.NewInlineKeyboardButtonData("Уточнить упражнения", CallbackAsk+"exercises"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Как отслеживать прогресс", CallbackAsk+"progress"),
				tgbotapi.NewInlineKeyboardButtonData("Что делать при диабете", CallbackAsk+"diabetes"),
			),
		)
		return &keyboard
	}

	return nil
}

// GetNextQuestion возвращает следующий вопрос в зависимости от текущего состояния
func (s *UserSession) GetNextQuestion() string {
	switch s.State {
	case StateAskSex:
		return "Укажите ваш пол:"
	case StateAskAge:
		return "Укажите ваш возраст (полных лет):"
	case StateAskHeight:
		return "Укажите ваш рост в сантиметрах (например, 175):"
	case StateAskWeight:
		return "Укажите ваш вес в килограммах (например, 70):"
	case StateAskDiabetes:
		return "У вас есть диабет?"
	case StateAskLevel:
		return "Оцените ваш текущий уровень физической подготовки:"
	case StateAskGoal:
		return "Какова ваша главная цель?"
	case StateAskType:
		return "Какой тип тренировок вы предпочитаете?"
	case StatePayment:
		return fmt.Sprintf("Спасибо! Ваша информация собрана:\n\n%s\n\nДля получения персональной программы тренировок, пожалуйста, оплатите услугу. Нажмите кнопку или введите /pay", s.Data.String())
	case StateComplete:
		return "Ваша персональная программа тренировок уже создана. Если вы хотите начать заново, используйте команду /start"
	default:
		return "Что-то пошло не так. Попробуйте начать сначала с команды /start"
	}
}

// GetAskQuestionAnswer возвращает ответ на вопрос о программе
// GetAskQuestionAnswer возвращает ответ на вопрос о программе
func (s *UserSession) GetAskQuestionAnswer(question string) string {
	switch question {
	case "nutrition":
		// Рекомендации по питанию
		baseText := "🍽️ **РЕКОМЕНДАЦИИ ПО ПИТАНИЮ**\n\n"

		weight := s.Data.Weight
		height := s.Data.Height
		goal := s.Data.FitnessGoal

		if goal == "похудение" {
			baseText += fmt.Sprintf("Для достижения вашей цели похудения, с учетом вашего веса %d кг и роста %d см, рекомендуется потреблять примерно %d-%d калорий в день, с дефицитом 400-500 калорий.\n\n",
				weight, height, (weight*30)-500, (weight*30)-400)
		} else if goal == "набор массы" {
			baseText += fmt.Sprintf("Для набора мышечной массы, с учетом вашего веса %d кг, рекомендуется потреблять примерно %d-%d калорий в день, с профицитом 300-400 калорий.\n\n",
				weight, (weight*30)+300, (weight*30)+400)
		} else {
			baseText += fmt.Sprintf("Для поддержания вашего текущего веса %d кг рекомендуется потреблять примерно %d-%d калорий в день.\n\n",
				weight, weight*28, weight*30)
		}

		baseText += "Рекомендуемое распределение макронутриентов:\n" +
			"- Белки: 1.6-2.0 г на кг веса тела (примерно " + fmt.Sprintf("%d-%d", int(float64(weight)*1.6), int(float64(weight)*2.0)) + " г в день)\n" +
			"- Жиры: 0.8-1.0 г на кг веса тела (примерно " + fmt.Sprintf("%d-%d", int(float64(weight)*0.8), int(float64(weight)*1.0)) + " г в день)\n" +
			"- Углеводы: оставшаяся часть калорий\n\n"

		baseText += "**Рекомендуемый режим питания:**\n" +
			"1. Завтрак: белковая пища + сложные углеводы (овсянка, яйца, нежирный творог)\n" +
			"2. Перекус: фрукт или протеиновый коктейль\n" +
			"3. Обед: белок + овощи + сложные углеводы (мясо/рыба, овощи, гречка/рис/киноа)\n" +
			"4. Перекус: орехи, йогурт или творог\n" +
			"5. Ужин (минимум за 2-3 часа до сна): белок + овощи (куриная грудка/рыба, овощной салат)\n\n"

		baseText += "**Рекомендации по питьевому режиму:**\n" +
			fmt.Sprintf("- Пейте не менее %d мл воды в день\n", weight*30) +
			"- Пейте стакан воды за 30 минут до каждого приема пищи\n" +
			"- Ограничьте потребление алкоголя и сладких напитков\n\n"

		if s.Data.Diabetes == "да" {
			baseText += "**Особые рекомендации при диабете:**\n" +
				"- Избегайте продуктов с высоким гликемическим индексом\n" +
				"- Контролируйте порции углеводов\n" +
				"- Равномерно распределяйте углеводы в течение дня\n" +
				"- Регулярно измеряйте уровень сахара в крови\n" +
				"- Проконсультируйтесь с эндокринологом для детального плана питания\n"
		}

		return baseText

	case "exercises":
		// Рекомендации по упражнениям
		baseText := "💪 **ПРОГРАММА УПРАЖНЕНИЙ**\n\n"
		fitnessType := s.Data.FitnessType

		if fitnessType == "силовые" {
			baseText += "**Силовая тренировка A (Понедельник):**\n" +
				"1. Разминка: 5-10 минут кардио и динамическая растяжка\n" +
				"2. Приседания: 4 подхода по 10-12 повторений\n" +
				"3. Жим лежа: 4 подхода по 8-10 повторений\n" +
				"4. Тяга в наклоне: 3 подхода по 10-12 повторений\n" +
				"5. Отжимания: 3 подхода до отказа\n" +
				"6. Планка: 3 подхода по 30-60 секунд\n" +
				"7. Растяжка: 5-10 минут\n\n"

			baseText += "**Силовая тренировка B (Четверг):**\n" +
				"1. Разминка: 5-10 минут кардио и динамическая растяжка\n" +
				"2. Становая тяга: 4 подхода по 8-10 повторений\n" +
				"3. Жим гантелей над головой: 3 подхода по 10-12 повторений\n" +
				"4. Подтягивания (или тяга верхнего блока): 3 подхода до отказа\n" +
				"5. Сгибания рук на бицепс: 3 подхода по 12-15 повторений\n" +
				"6. Разгибания рук на трицепс: 3 подхода по 12-15 повторений\n" +
				"7. Растяжка: 5-10 минут\n\n"
		} else if fitnessType == "кардио" {
			baseText += "**Кардио тренировка (Вторник, Пятница):**\n" +
				"1. Разминка: 5 минут легкой ходьбы или медленного бега\n" +
				"2. Интервальная тренировка: 30 секунд спринта + 90 секунд ходьбы (повторить 10 раз)\n" +
				"3. Заминка: 5 минут медленной ходьбы\n\n"

			baseText += "**HIIT тренировка (Суббота):**\n" +
				"1. Разминка: 5 минут\n" +
				"2. Круговая тренировка (без отдыха между упражнениями, 60 сек отдыха между кругами):\n" +
				"   - Бёрпи: 30 секунд\n" +
				"   - Приседания с выпрыгиванием: 30 секунд\n" +
				"   - Альпинист: 30 секунд\n" +
				"   - Скручивания: 30 секунд\n" +
				"   - Прыжки со скакалкой: 60 секунд\n" +
				"3. Повторите круг 3-5 раз\n" +
				"4. Заминка и растяжка: 5-10 минут\n\n"
		} else {
			baseText += "**Тренировка всего тела (3 раза в неделю - пн, ср, пт):**\n" +
				"1. Разминка: 5-10 минут кардио и динамическая растяжка\n" +
				"2. Приседания: 3 подхода по 12-15 повторений\n" +
				"3. Отжимания: 3 подхода по 10-12 повторений\n" +
				"4. Гиперэкстензия: 3 подхода по 12-15 повторений\n" +
				"5. Планка: 3 подхода по 30-60 секунд\n" +
				"6. Кардио: 15-20 минут (бег, велосипед, эллипс)\n" +
				"7. Растяжка: 5-10 минут\n\n"
		}

		baseText += "**Общие рекомендации:**\n" +
			"- Всегда начинайте с разминки, чтобы избежать травм\n" +
			"- Контролируйте правильную технику выполнения упражнений\n" +
			"- Постепенно увеличивайте нагрузку каждые 2-3 недели\n" +
			"- Если чувствуете боль (не путать с мышечной усталостью), прекратите выполнение упражнения\n" +
			"- Делайте 1-2 дня отдыха в неделю для восстановления\n\n"

		if s.Data.Level == "начинающий" {
			baseText += "**Рекомендации для начинающих:**\n" +
				"- Начните с меньшего веса и меньшего количества повторений\n" +
				"- Сосредоточьтесь на изучении правильной техники\n" +
				"- Увеличивайте нагрузку постепенно\n"
		}

		return baseText

	case "progress":
		// Рекомендации по отслеживанию прогресса
		return "📊 **КАК ОТСЛЕЖИВАТЬ ПРОГРЕСС**\n\n" +
			"**Основные метрики для отслеживания:**\n" +
			"1. **Вес** - взвешивайтесь 1-2 раза в неделю, в одно и то же время (лучше утром натощак)\n" +
			"2. **Замеры тела** - делайте замеры основных частей тела раз в 2-4 недели:\n" +
			"   - Окружность шеи\n" +
			"   - Окружность груди\n" +
			"   - Окружность талии\n" +
			"   - Окружность бедер\n" +
			"   - Окружность бицепса\n" +
			"   - Окружность бедра\n" +
			"   - Окружность икры\n" +
			"3. **Фотографии** - делайте фото в одинаковых условиях (освещение, поза, одежда) раз в 4 недели\n" +
			"4. **Тренировочный дневник** - записывайте веса и повторения для каждого упражнения\n" +
			"5. **Пищевой дневник** - отслеживайте потребляемые калории и макронутриенты\n\n" +
			"**Дополнительные параметры:**\n" +
			"- **Энергия и самочувствие** - оценивайте по шкале от 1 до 10\n" +
			"- **Качество сна** - продолжительность и ощущение отдыха после сна\n" +
			"- **Работоспособность на тренировках** - насколько легко/трудно выполнять упражнения\n\n" +
			"**Технологии для отслеживания:**\n" +
			"- Приложения для подсчета калорий (MyFitnessPal, FatSecret)\n" +
			"- Приложения для тренировок (Strong, Jefit, Nike Training Club)\n" +
			"- Фитнес-трекеры и умные часы для отслеживания активности\n\n" +
			"**Как оценивать результаты:**\n" +
			"- При похудении: ожидайте потерю 0.5-1 кг в неделю (безопасная норма)\n" +
			"- При наборе массы: 0.2-0.5 кг в неделю может считаться хорошим результатом\n" +
			"- Обращайте внимание на изменение размеров тела и самочувствие\n" +
			"- Если прогресс остановился на 2-3 недели, пересмотрите программу и питание\n\n" +
			"**Важно помнить:**\n" +
			"- Прогресс редко бывает линейным\n" +
			"- На вес влияют многие факторы (вода, соль, гормоны, стресс)\n" +
			"- Оценивайте прогресс комплексно, а не только по весу\n" +
			"- Будьте терпеливы - устойчивые результаты требуют времени"

	case "diabetes":
		// Рекомендации при диабете
		diabetesText := "🩺 **ТРЕНИРОВКИ ПРИ ДИАБЕТЕ**\n\n"

		if s.Data.Diabetes == "да" {
			diabetesText += "**Основные рекомендации для тренировок при диабете:**\n\n" +
				"**Перед тренировкой:**\n" +
				"- Измерьте уровень глюкозы в крови перед тренировкой\n" +
				"- Если уровень ниже 5.6 ммоль/л, съешьте небольшую порцию углеводов\n" +
				"- Если уровень выше 13.9 ммоль/л, отложите интенсивную тренировку\n" +
				"- Имейте с собой быстрые углеводы (глюкоза, сок) на случай гипогликемии\n" +
				"- Носите идентификационный браслет диабетика\n\n" +
				"**Во время тренировки:**\n" +
				"- Обращайте внимание на симптомы гипогликемии (дрожь, слабость, головокружение, потливость)\n" +
				"- При длительных тренировках (более 45-60 минут) периодически проверяйте уровень сахара\n" +
				"- Пейте достаточно воды\n\n" +
				"**После тренировки:**\n" +
				"- Измерьте уровень глюкозы после тренировки\n" +
				"- Будьте внимательны к отсроченной гипогликемии (может возникнуть через 4-48 часов)\n" +
				"- Убедитесь, что у вас есть план питания после тренировки\n\n" +
				"**Выбор типа тренировок:**\n" +
				"- Комбинируйте кардио и силовые тренировки для лучшего контроля сахара\n" +
				"- Начинайте с низкой интенсивности и постепенно увеличивайте нагрузку\n" +
				"- Силовые тренировки улучшают чувствительность к инсулину\n" +
				"- Аэробные тренировки умеренной интенсивности (ходьба, плавание, велосипед) хорошо подходят для контроля сахара\n\n" +
				"**Коррекция дозы инсулина и лекарств:**\n" +
				"- Проконсультируйтесь с вашим эндокринологом относительно коррекции дозы инсулина или лекарств перед тренировкой\n" +
				"- Обычно требуется снижение дозы инсулина перед физической активностью\n\n" +
				"**Важно:**\n" +
				"- Всегда консультируйтесь с врачом перед началом новой программы тренировок\n" +
				"- Ведите дневник, отмечая уровень сахара до, во время и после тренировок\n" +
				"- Будьте особенно внимательны при тренировках в жару или холод\n" +
				"- Следите за состоянием ног и используйте подходящую обувь\n"
		} else {
			diabetesText += "У вас не указан диабет, но вот общие рекомендации, которые полезно знать всем:\n\n" +
				"1. Регулярные физические нагрузки помогают поддерживать нормальный уровень сахара в крови\n" +
				"2. Сбалансированное питание с контролем потребления простых углеводов поддерживает здоровый метаболизм\n" +
				"3. Следите за ощущениями во время тренировок - слабость, головокружение, повышенная жажда могут указывать на проблемы с сахаром\n" +
				"4. Регулярное медицинское обследование поможет выявить потенциальные проблемы на ранней стадии\n\n" +
				"Профилактика диабета 2 типа:\n" +
				"- Поддерживайте здоровый вес\n" +
				"- Регулярно занимайтесь физическими упражнениями\n" +
				"- Ешьте здоровую пищу, богатую клетчаткой и низкую по гликемическому индексу\n" +
				"- Ограничьте потребление сахара и рафинированных углеводов\n" +
				"- Избегайте курения и чрезмерного употребления алкоголя\n"
		}

		return diabetesText

	default:
		return "Пожалуйста, уточните ваш вопрос о программе тренировок."
	}
}

// ProcessButtonCallback обрабатывает нажатие на кнопку
func (s *UserSession) ProcessButtonCallback(data string) (string, error) {
	if len(data) < 4 {
		return "Некорректные данные", fmt.Errorf("некорректные данные callback: %s", data)
	}

	// Обработка вопросов о программе тренировок
	if strings.HasPrefix(data, "ask_") {
		question := strings.TrimPrefix(data, "ask_")
		return s.GetAskQuestionAnswer(question), nil
	}

	var prefix, value string

	// Определяем префикс и значение
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
		return "Неизвестная команда", fmt.Errorf("неизвестный префикс в callback: %s", data)
	}

	// Обработка в зависимости от префикса
	switch prefix {
	case CallbackSex:
		s.Data.Sex = map[string]string{
			"male":   "мужской",
			"female": "женский",
		}[value]
		s.State = StateAskAge

	case CallbackDiabetes:
		s.Data.Diabetes = map[string]string{
			"yes": "да",
			"no":  "нет",
		}[value]
		s.State = StateAskLevel

	case CallbackLevel:
		s.Data.Level = map[string]string{
			"beginner":     "начинающий",
			"intermediate": "средний",
			"advanced":     "продвинутый",
		}[value]
		s.State = StateAskGoal

	case CallbackGoal:
		s.Data.FitnessGoal = map[string]string{
			"weight_loss": "похудение",
			"muscle_gain": "набор массы",
			"maintenance": "поддержание формы",
			"endurance":   "улучшение выносливости",
		}[value]
		s.State = StateAskType

	case CallbackType:
		s.Data.FitnessType = map[string]string{
			"strength": "силовые",
			"cardio":   "кардио",
			"mixed":    "смешанные",
			"yoga":     "йога",
			"pilates":  "пилатес",
			"other":    "другое",
		}[value]
		s.State = StatePayment
	}

	// Возвращаем следующий вопрос
	return s.GetNextQuestion(), nil
}

// ProcessInput обрабатывает ввод пользователя на основе текущего состояния
func (s *UserSession) ProcessInput(input string) (string, error) {
	switch s.State {
	case StateInitial:
		s.State = StateAskSex
		return "Давайте создадим для вас персональную фитнес-программу! Сначала я задам несколько вопросов.\n\nУкажите ваш пол:", nil

	case StateAskSex:
		// Если ввод текстом, а не через кнопки
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
		return "У вас есть диабет?", nil

	case StateAskDiabetes:
		// Если ввод текстом, а не через кнопки
		s.Data.Diabetes = input
		s.State = StateAskLevel
		return "Оцените ваш текущий уровень физической подготовки:", nil

	case StateAskLevel:
		// Если ввод текстом, а не через кнопки
		s.Data.Level = input
		s.State = StateAskGoal
		return "Какова ваша главная цель?", nil

	case StateAskGoal:
		// Если ввод текстом, а не через кнопки
		s.Data.FitnessGoal = input
		s.State = StateAskType
		return "Какой тип тренировок вы предпочитаете?", nil

	case StateAskType:
		// Если ввод текстом, а не через кнопки
		s.Data.FitnessType = input
		s.State = StatePayment
		return fmt.Sprintf("Спасибо! Ваша информация собрана:\n\n%s\n\nДля получения персональной программы тренировок, пожалуйста, оплатите услугу. Введите /pay", s.Data.String()), nil

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
