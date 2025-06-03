package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	maxExpressionLength = 100
	requestTimeout      = 30 * time.Second
)

// Calculator представляет калькулятор с расширенными возможностями
type Calculator struct {
	supportedOps map[string]func(float64, float64) (float64, error)
}

// NewCalculator создает новый экземпляр калькулятора
func NewCalculator() *Calculator {
	return &Calculator{
		supportedOps: map[string]func(float64, float64) (float64, error){
			"+": func(a, b float64) (float64, error) { return a + b, nil },
			"-": func(a, b float64) (float64, error) { return a - b, nil },
			"*": func(a, b float64) (float64, error) { return a * b, nil },
			"×": func(a, b float64) (float64, error) { return a * b, nil },
			"/": func(a, b float64) (float64, error) {
				if b == 0 {
					return 0, fmt.Errorf("деление на ноль")
				}
				return a / b, nil
			},
			"÷": func(a, b float64) (float64, error) {
				if b == 0 {
					return 0, fmt.Errorf("деление на ноль")
				}
				return a / b, nil
			},
			"^":  func(a, b float64) (float64, error) { return math.Pow(a, b), nil },
			"**": func(a, b float64) (float64, error) { return math.Pow(a, b), nil },
			"%": func(a, b float64) (float64, error) {
				if b == 0 {
					return 0, fmt.Errorf("деление на ноль при вычислении остатка")
				}
				return math.Mod(a, b), nil
			},
		},
	}
}

// validateExpression проверяет корректность выражения
func (c *Calculator) validateExpression(expr string) error {
	if len(expr) > maxExpressionLength {
		return fmt.Errorf("выражение слишком длинное (максимум %d символов)", maxExpressionLength)
	}

	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("пустое выражение")
	}

	// Проверка на подозрительные символы
	validChars := regexp.MustCompile(`^[0-9+\-*/×÷^%().\s]+$`)
	if !validChars.MatchString(expr) {
		return fmt.Errorf("выражение содержит недопустимые символы")
	}

	return nil
}

// parseExpression разбирает выражение и находит операцию
func (c *Calculator) parseExpression(expr string) (float64, string, float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")

	// Поиск операторов в порядке приоритета (сначала двухсимвольные)
	operators := []string{"**", "÷", "×", "^", "%", "/", "*", "+", "-"}

	for _, op := range operators {
		// Для операторов - и +, нужно учитывать, что они могут быть знаками числа
		if op == "-" || op == "+" {
			// Ищем операторы не в начале строки и не после другого операторного символа
			for i := 1; i < len(expr); i++ {
				if string(expr[i]) == op {
					// Проверяем, что это не знак числа
					prevChar := expr[i-1]
					if prevChar >= '0' && prevChar <= '9' || prevChar == ')' {
						left := expr[:i]
						right := expr[i+1:]
						if right != "" {
							a, err1 := strconv.ParseFloat(left, 64)
							b, err2 := strconv.ParseFloat(right, 64)
							if err1 == nil && err2 == nil {
								return a, op, b, nil
							}
						}
					}
				}
			}
		} else {
			// Для других операторов просто ищем первое вхождение
			if idx := strings.Index(expr, op); idx > 0 {
				left := expr[:idx]
				right := expr[idx+len(op):]
				if right != "" {
					a, err1 := strconv.ParseFloat(left, 64)
					b, err2 := strconv.ParseFloat(right, 64)
					if err1 == nil && err2 == nil {
						return a, op, b, nil
					}
				}
			}
		}
	}

	return 0, "", 0, fmt.Errorf("операция не найдена или неправильный формат")
}

// Calculate выполняет вычисление выражения
func (c *Calculator) Calculate(expr string) (string, error) {
	if err := c.validateExpression(expr); err != nil {
		return "", err
	}

	a, op, b, err := c.parseExpression(expr)
	if err != nil {
		return "", err
	}

	opFunc, exists := c.supportedOps[op]
	if !exists {
		return "", fmt.Errorf("неподдерживаемая операция: %s", op)
	}

	result, err := opFunc(a, b)
	if err != nil {
		return "", err
	}

	// Проверка на переполнение и NaN
	if math.IsInf(result, 0) {
		return "", fmt.Errorf("результат слишком велик")
	}
	if math.IsNaN(result) {
		return "", fmt.Errorf("результат не является числом")
	}

	// Форматирование результата
	if result == float64(int64(result)) {
		return fmt.Sprintf("%.0f", result), nil
	}
	return fmt.Sprintf("%.6g", result), nil
}

// BotHandler обрабатывает сообщения бота
type BotHandler struct {
	bot        *tgbotapi.BotAPI
	calculator *Calculator
}

// NewBotHandler создает новый обработчик бота
func NewBotHandler(bot *tgbotapi.BotAPI) *BotHandler {
	return &BotHandler{
		bot:        bot,
		calculator: NewCalculator(),
	}
}

// handleMessage обрабатывает входящее сообщение
func (h *BotHandler) handleMessage(message *tgbotapi.Message) {
	if message == nil || message.Text == "" {
		return
	}

	var reply string
	text := strings.TrimSpace(message.Text)

	// Команды бота
	switch {
	case text == "/start":
		reply = `👋 Привет! Я калькулятор-бот.
		
Поддерживаемые операции:
• Сложение: +
• Вычитание: -
• Умножение: * или ×
• Деление: / или ÷
• Возведение в степень: ^ или **
• Остаток от деления: %

Примеры:
• 2 + 3
• 10.5 * 2
• 16 / 4
• 2 ^ 3
• 10 % 3

Просто отправьте мне математическое выражение!`

	case text == "/help":
		reply = `📖 Справка по использованию:

Отправьте математическое выражение в формате: число операция число

Примеры корректных выражений:
• 15 + 25
• 100 - 50
• 12.5 * 4
• 144 / 12
• 2 ^ 10
• 17 % 5

⚠️ Ограничения:
• Максимум 100 символов
• Только простые выражения (два числа и одна операция)
• Деление на ноль запрещено`

	default:
		// Попытка вычислить выражение
		result, err := h.calculator.Calculate(text)
		if err != nil {
			reply = "❌ Ошибка: " + err.Error() + "\n\nИспользуйте /help для получения справки."
		} else {
			reply = "✅ Результат: " + result
		}
	}

	// Отправка ответа
	msg := tgbotapi.NewMessage(message.Chat.ID, reply)
	msg.ReplyToMessageID = message.MessageID

	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}

// Start запускает обработку сообщений
func (h *BotHandler) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := h.bot.GetUpdatesChan(u)

	log.Println("🤖 Бот запущен и готов к работе!")

	for {
		select {
		case <-ctx.Done():
			log.Println("📴 Получен сигнал остановки, завершаем работу...")
			h.bot.StopReceivingUpdates()
			return ctx.Err()

		case update := <-updates:
			// Обработка в отдельной горутине для неблокирующей работы
			go func(upd tgbotapi.Update) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Паника при обработке сообщения: %v", r)
					}
				}()

				h.handleMessage(upd.Message)
			}(update)
		}
	}
}

func main() {
	// Настройка логирования
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Получение токена бота
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		// Fallback для разработки (уберите в продакшене!)
		botToken = "7566241176:AAHIsMArqeqDEM8LxDv-9Rvh5zPmQCxa2a4"
		log.Println("⚠️  Используется токен по умолчанию. Рекомендуется установить TELEGRAM_BOT_TOKEN")
	}

	// Создание бота
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("❌ Ошибка создания бота: %v", err)
	}

	// Настройка отладки (можно включить через переменную окружения)
	if os.Getenv("DEBUG") == "true" {
		bot.Debug = true
	}

	log.Printf("✅ Авторизован как @%s", bot.Self.UserName)

	// Создание обработчика
	handler := NewBotHandler(bot)

	// Настройка graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Канал для получения сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запуск бота в отдельной горутине
	go func() {
		if err := handler.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("❌ Ошибка работы бота: %v", err)
		}
	}()

	// Ожидание сигнала завершения
	<-sigChan
	log.Println("📴 Получен сигнал завершения...")
	cancel()

	// Ожидание завершения всех горутин
	time.Sleep(2 * time.Second)
	log.Println("👋 Бот остановлен")
}
