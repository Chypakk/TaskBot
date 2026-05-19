package scheduler

import (
	"context"
	"log"
	"time"

	"tg_sheduler/internal/domain"
	"tg_sheduler/internal/repository"
)

type Scheduler struct {
	repo     repository.NotificationRepository
	interval time.Duration
	notifier domain.Notifier
}

func New(repo repository.NotificationRepository, interval time.Duration, notifier domain.Notifier) *Scheduler {
	return &Scheduler{
		repo:     repo,
		interval: interval,
		notifier: notifier,
	}
}

// Start запускает фоновый цикл. Блокирует горутину до отмены контекста.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	log.Println("⏱️ Scheduler started, interval:", s.interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopped gracefully")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	// 1. Забираем пачку готовых уведомлений
	notifs, err := s.repo.GetNotifications(ctx)
	if err != nil {
		log.Printf("Scheduler tick error (fetch): %v", err)
		return
	}
	if len(notifs) == 0 {
		return // Нечего отправлять
	}

	log.Printf("Found %d pending notifications", len(notifs))

	// 2. Обрабатываем каждое
	for _, n := range notifs {
		if err := s.notifier.Notify(ctx, n.UserID, n.TaskID, n.Label, n.Time.Format("15:04 02.01")); err != nil {
			log.Printf("⚠️ Failed to notify user %d: %v", n.UserID, err)
			// Не помечаем как sent, чтобы попробовать снова на следующем тике
			continue
		}

		// 3. Помечаем как отправленное
		if err := s.repo.MarkAsSent(ctx, n.ID); err != nil {
			// Логируем, но не прерываем цикл. Повторная попытка на следующем тике.
			log.Printf("️ Failed to mark notif %d as sent: %v", n.ID, err)
		}
	}
}
