package scheduler

import (
	"context"
	"fmt"
	"tg_sheduler/internal/repository"

	tele "gopkg.in/telebot.v4"
)

type TelegramNotifier struct {
	bot      *tele.Bot
	taskRepo repository.TaskRepository
}

func NewTelegramNotifier(token string, taskRepo repository.TaskRepository) (*TelegramNotifier, error) {
	poller := &tele.LongPoller{Timeout: 10}

	bot, err := tele.NewBot(tele.Settings{
		Token:     token,
		Poller:    poller,
		ParseMode: tele.ModeMarkdown,
	})
	if err != nil {
		return nil, fmt.Errorf("telebot init: %w", err)
	}

	return &TelegramNotifier{bot: bot, taskRepo: taskRepo}, nil
}

func (n *TelegramNotifier) Notify(ctx context.Context, userID, taskID int, label, timeStr string) error {
	task, err := n.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	msg := fmt.Sprintf(
		"🔔 *Задача #%d*\n"+
			"📌 %s\n"+
			"⏰ %s",
		taskID, task.Name, timeStr,
	)

	// Отправляем пользователю по ID
	_, err = n.bot.Send(&tele.User{ID: int64(userID)}, msg)
	if err != nil {
		return fmt.Errorf("send telegram: %w", err)
	}
	return nil
}

func (n *TelegramNotifier) GetBot() *tele.Bot {
	return n.bot
}

func (n *TelegramNotifier) Start() {
	n.bot.Start()
}

func (n *TelegramNotifier) Stop() {
	n.bot.Stop()
}
