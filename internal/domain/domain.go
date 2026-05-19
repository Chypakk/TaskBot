package domain

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Importance int

const (
	Low Importance = iota
	Medium
	High
)

type User struct {
	ID       int
	TG_ID    int
	Username string
}

type Task struct {
	ID           int
	Importance   int
	UserID       int
	Name         string
	StartDate    time.Time
	DurationDays int
}

type Notification struct {
	ID        int
	TaskID    int
	UserID    int
	Time      time.Time
	Label     string
	IsSent    bool
	CreatedAt time.Time
}

type Notifier interface {
    Notify(ctx context.Context, userID, taskID int, label, timeStr string) error
}

type TaskCalculator struct {
	location *time.Location
}

func NewTaskCalculator(loc *time.Location) *TaskCalculator {
	return &TaskCalculator{location: loc}
}

// парсинг формата "dd.mm days", возвращает дату начала, кол-во дней
func (tc *TaskCalculator) ParseTaskInput(input string) (time.Time, int, error) {
	parts := strings.Split(strings.TrimSpace(input), " ")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("формат: dd.mm days")
	}

	dateStr := fmt.Sprintf("%s.%d", parts[0], time.Now().Year())
	start, err := time.ParseInLocation("02.01.2006", dateStr, tc.location)
	if err != nil {
		return time.Time{}, 0, err
	}

	days, err := strconv.Atoi(parts[1])
	if err != nil || days < 1 {
		return time.Time{}, 0, fmt.Errorf("days >= 1")
	}

	return start, days, nil
}

// генерация уведомлений
func (tc *TaskCalculator) CalculateNotifications(start time.Time, days int, imp Importance) []Notification {
	var notifs []Notification
	isSingle := days == 1
	loc := tc.location

	// помощник для создания времени в нужный час того же дня
	atHour := func(t time.Time, h int) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), h, 0, 0, 0, loc)
	}

	switch imp {
	case Low:
		if isSingle {
			notifs = append(notifs,
				Notification{Time: atHour(start, 8), Label: "начало дня"},
				Notification{Time: atHour(start, 18), Label: "конец дня"},
			)
		} else {
			// следующий день от начала + день до конца
			notifs = append(notifs,
				Notification{Time: atHour(start.AddDate(0, 0, 1), 12), Label: "середина дня (после старта)"},
				Notification{Time: atHour(start.AddDate(0, 0, days-2), 12), Label: "середина дня (до конца)"},
			)
		}

	case Medium:
		if isSingle {
			for _, h := range []int{8, 12, 18} {
				notifs = append(notifs, Notification{Time: atHour(start, h), Label: "фикс"})
			}
		} else {
			// Делим промежуток на 3 условные части и берём репрезентативный день для каждой
			step := int(math.Ceil(float64(days) / 3.0))
			daysToNotify := []int{1, 2*step - 1, days - 1} // 0-based индексы
			seen := make(map[int]bool)

			for _, idx := range daysToNotify {
				if idx >= days {
					idx = days - 1
				}
				if idx < 0 {
					idx = 0
				}
				if seen[idx] {
					continue
				}
				seen[idx] = true
				d := start.AddDate(0, 0, idx)
				notifs = append(notifs,
					Notification{Time: atHour(d, 8), Label: "утро"},
					Notification{Time: atHour(d, 18), Label: "вечер"},
				)
			}
		}

	case High:
		if isSingle {
			// каждые 2 часа
			for h := 8; h <= 20; h += 2 {
				notifs = append(notifs, Notification{Time: atHour(start, h), Label: "каждые 2ч"})
			}
		} else {
			for i := 1; i < days; i++ {
				d := start.AddDate(0, 0, i)
				for _, h := range []int{8, 12, 18} {
					notifs = append(notifs, Notification{Time: atHour(d, h), Label: "фикс"})
				}
			}
		}
	}

	// Сортируем хронологически
	sort.Slice(notifs, func(i, j int) bool {
		return notifs[i].Time.Before(notifs[j].Time)
	})

	return notifs
}
