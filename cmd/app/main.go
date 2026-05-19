package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"tg_sheduler/internal/domain"
	"tg_sheduler/internal/infrastrucutre"
	"tg_sheduler/internal/repository"
	"tg_sheduler/internal/scheduler"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 1. БД
	db, err := infrastrucutre.NewSqliteStorage()
	if err != nil {
		log.Fatalf("DB init: %v", err)
	}
	defer db.Close()

	repo := repository.NewSQLiteRepo(db)

	// 2. Тестовые данные (можно убрать потом)
	user, _ := repo.CreateUser(ctx, &domain.User{TG_ID: 123456, Username: "test"})
	loc, _ := time.LoadLocation("Europe/Moscow")
	calc := domain.NewTaskCalculator(loc)
	start, days, _ := calc.ParseTaskInput("18.05 3") // Вчерашняя дата для мгновенного триггера

	task := &domain.Task{
		UserID: user.ID, Name: "Тестовая задача", Importance: int(domain.High),
		StartDate: start, DurationDays: days,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		log.Fatalf("CreateTask: %v", err)
	}

	notifs := calc.CalculateNotifications(start, days, domain.Importance(task.Importance))
	for i := range notifs {
		notifs[i].TaskID = task.ID
		notifs[i].UserID = user.ID
	}
	if err := repo.CreateBatch(ctx, notifs); err != nil {
		log.Fatalf("CreateNotifs: %v", err)
	}
	fmt.Println("✅ Test data inserted")

	// 3. Запуск шедулера
	sched := scheduler.New(repo, 10*time.Second, new(ConsoleNotifier)) // Для теста каждые 10 сек
	go sched.Start(ctx)

	// 4. Ждём сигнала выхода
	<-ctx.Done()
	log.Println("👋 App shutting down...")
}

type ConsoleNotifier struct{}

func (n *ConsoleNotifier) Notify(ctx context.Context, userID, taskID int, label, timeStr string) error {
    log.Printf("[MOCK SEND] To %d: %d [%s] at %s", userID, taskID, label, timeStr)
    return nil
}