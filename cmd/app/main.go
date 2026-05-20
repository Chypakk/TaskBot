package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"tg_sheduler/internal/bot"
	"tg_sheduler/internal/domain"
	"tg_sheduler/internal/infrastrucutre"
	"tg_sheduler/internal/repository"
	"tg_sheduler/internal/scheduler"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	envPath := findEnvFile()
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("⚠️ .env not loaded: %v", err)
	}

	// 1. БД + репо
	db, err := infrastrucutre.NewSqliteStorage()
	if err != nil {
		log.Fatalf("🗄️ DB init: %v", err)
	}
	defer db.Close()
	repo := repository.NewSQLiteRepo(db)

	// 2. Токен
	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("❌ TOKEN not set in .env")
	}

	// 3. Таймзона (можно вынести в конфиг)
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Printf("⚠️ Failed to load timezone, using local: %v", err)
		loc = time.Now().Location()
	}

	// 4. Бот + FSM + калькулятор
	tgNotif, err := scheduler.NewTelegramNotifier(token, repo)
	if err != nil {
		log.Fatalf("🤖 Bot init: %v", err)
	}

	fsm := bot.NewFSM()
	calc := domain.NewTaskCalculator(loc)
	handlers := bot.NewHandlers(repo, repo, repo, fsm, calc, loc)

	botInstance := tgNotif.GetBot()

	// Регистрация хендлеров
	botInstance.Handle("/start", handlers.Start)
	botInstance.Handle("/cancel", handlers.Cancel)
	botInstance.Handle(tele.OnCallback, handlers.Callback)
	botInstance.Handle(tele.OnText, handlers.HandleText)

	// 5. Шедулер уведомлений
	sched := scheduler.New(repo, repo, 10*time.Second, tgNotif)
	go sched.Start(ctx)

	// 6. Запуск бота
	go tgNotif.Start()
	log.Println("🚀 Bot is running...")

	<-ctx.Done()
	tgNotif.Stop()
	log.Println("👋 Graceful shutdown")
}

type ConsoleNotifier struct{}

func (n *ConsoleNotifier) Notify(ctx context.Context, userID, taskID int, label, timeStr string) error {
    log.Printf("[MOCK SEND] To %d: %d [%s] at %s", userID, taskID, label, timeStr)
    return nil
}

// поиск env файла
func findEnvFile() string {
	// Рядом с exe
	if exePath, err := os.Executable(); err == nil {
		dir := filepath.Dir(exePath)
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// В текущей рабочей директории
	if cwd, err := os.Getwd(); err == nil {
		envPath := filepath.Join(cwd, ".env")
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	return ""
}