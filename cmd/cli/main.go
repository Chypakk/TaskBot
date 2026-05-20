package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"tg_sheduler/internal/bot"
	"tg_sheduler/internal/domain"
	"tg_sheduler/internal/infrastrucutre"
	"tg_sheduler/internal/repository"
)

const (
	TestTGID     = 88888
	TestUsername = "integration_tester"
)

func main() {
	// 1. Реальная БД + репозитории
	db, err := infrastrucutre.NewSqliteStorage()
	if err != nil {
		log.Fatalf("🗄️ Ошибка инициализации БД: %v", err)
	}
	defer db.Close()

	// Чистим старые тестовые данные, чтобы каждый запуск был "с чистого листа"
	cleanupTestDB(db)

	repo := repository.NewSQLiteRepo(db)
	loc, _ := time.LoadLocation("Europe/Moscow")
	fsm := bot.NewFSM()
	calc := domain.NewTaskCalculator(loc)
	ctx := context.Background()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("🧪 Интеграционный тест: FSM + Repository")
	fmt.Println("📝 Сценарий: /start → создание → сохранение → лист → редактирование → verify")
	fmt.Println("💡 Вводи команды по порядку или exit для выхода:")
	fmt.Println("   /start          — инициализация юзера и сброс FSM")
	fmt.Println("   create          — начать создание задачи (переход в StateCreatingName)")
	fmt.Println("   [текст]         — ввод в зависимости от текущего состояния FSM")
	fmt.Println("   list            — вывести задачи из БД")
	fmt.Println("   edit <id>       — начать редактирование задачи")
	fmt.Println("   verify          — глубокая проверка текущего состояния БД и FSM")
	fmt.Println("   exit            — выход")
	fmt.Println()

	for {
		printFSMState(fsm)
		fmt.Print("👉 Команда: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" || input == "exit" {
			break
		}

		switch {
		case input == "/start":
			runStart(ctx, fsm, repo)
		case input == "create":
			runCreateStart(fsm)
		case input == "list":
			runListTasks(ctx, repo)
		case strings.HasPrefix(input, "edit "):
			idStr := strings.TrimPrefix(input, "edit ")
			runEditStart(ctx, fsm, repo, idStr)
		case input == "verify":
			runVerify(ctx, repo, fsm)
		default:
			// Текстовый ввод → обрабатываем по текущему состоянию FSM
			runTextInput(ctx, fsm, repo, calc, input)
		}
	}
}

// === Обработчики команд (копируют логику из handlers.go, но без tele.Context) ===

func runStart(ctx context.Context, fsm *bot.FSM, repo *repository.SQLiteRepo) {
	fsm.Reset(TestTGID)
	// Авто-регистрация юзера (как в handlers.Start)
	_, err := repo.CreateUser(ctx, &domain.User{TG_ID: TestTGID, Username: TestUsername})
	if err != nil {
		fmt.Printf("⚠️ Ошибка создания юзера: %v\n", err)
		return
	}
	fmt.Println("✅ /start выполнен: юзер создан/обновлён, FSM сброшена в idle")
}

func runCreateStart(fsm *bot.FSM) {
	fsm.Set(TestTGID, bot.FSMState{State: bot.StateCreatingName})
	fmt.Println("🔄 FSM: idle → creating:name")
	fmt.Println("💬 Ожидание: введите название задачи")
}

func runTextInput(ctx context.Context, fsm *bot.FSM, repo *repository.SQLiteRepo, calc *domain.TaskCalculator, text string) {
	state, ok := fsm.Get(TestTGID)
	if !ok {
		fmt.Println("⚠️ Нет активного состояния. Начни с /start или create")
		return
	}

	switch state.State {
	case bot.StateCreatingName:
		if text == "" {
			fmt.Println("❌ Название не может быть пустым")
			return
		}
		fsm.Set(TestTGID, bot.FSMState{State: bot.StateCreatingDate, Payload: text})
		fmt.Printf("🔄 FSM: creating:name → creating:date\n")
		fmt.Printf("💾 Временно сохранено название: \"%s\"\n", text)
		fmt.Println("💬 Ожидание: введите дату в формате dd.mm N (напр. 21.05 3)")

	case bot.StateCreatingDate:
		start, days, err := calc.ParseTaskInput(text)
		if err != nil {
			fmt.Printf("❌ Ошибка парсинга: %v\n", err)
			return
		}
		// Берём название из предыдущего шага
		name := state.Payload
		payload := fmt.Sprintf("%s|%s|%d", name, start.Format(time.RFC3339), days)
		fsm.Set(TestTGID, bot.FSMState{State: bot.StateCreatingImportance, Payload: payload})
		fmt.Printf("🔄 FSM: creating:date → creating:importance\n")
		fmt.Printf("💾 Временные данные: name=%q, start=%s, days=%d\n", name, start.Format("02.01.2006"), days)
		fmt.Println("💬 Ожидание: введите важность (1/2/3)")

	case bot.StateCreatingImportance:
		imp, err := strconv.Atoi(text)
		if err != nil || imp < 1 || imp > 3 {
			fmt.Println("❌ Важность должна быть 1, 2 или 3")
			return
		}
		// Десериализуем payload как в handleCreateWithImportance
		parts := strings.SplitN(state.Payload, "|", 3)
		if len(parts) != 3 {
			fmt.Println("❌ Внутренняя ошибка данных")
			return
		}
		name := parts[0]
		start, _ := time.Parse(time.RFC3339, parts[1])
		days, _ := strconv.Atoi(parts[2])

		// Сохраняем задачу в БД
		user, _ := repo.GetUserByTGID(ctx, TestTGID)
		task := &domain.Task{
			UserID:       user.ID,
			Name:         name,
			Importance:   imp,
			StartDate:    start,
			DurationDays: days,
		}
		if err := repo.Create(ctx, task); err != nil {
			fmt.Printf("❌ Ошибка сохранения задачи: %v\n", err)
			return
		}

		// Генерируем уведомления (как в реальном хендлере)
		notifs := calc.CalculateNotifications(start, days, domain.Importance(imp))
		for i := range notifs {
			notifs[i].TaskID = task.ID
			notifs[i].UserID = user.ID
		}
		// Можно сохранить уведомления, но для теста опустим, чтобы не перегружать вывод
		// _ = repo.CreateBatch(ctx, notifs)

		fsm.Reset(TestTGID)
		fmt.Println("✅ Задача успешно сохранена в БД!")
		fmt.Printf("   📌 ID: %d | Название: %q | Важность: %d | Старт: %s | Дней: %d\n",
			task.ID, task.Name, task.Importance, task.StartDate.Format("02.01.2006"), task.DurationDays)
		fmt.Println("🔄 FSM: → idle")

	case bot.StateEditingName:
		task, _ := repo.GetByID(ctx, state.TaskID)
		task.Name = text
		if err := repo.Update(ctx, task); err != nil {
			fmt.Printf("❌ Ошибка обновления: %v\n", err)
			return
		}
		fsm.Reset(TestTGID)
		fmt.Println("✅ Название обновлено в БД")
		fmt.Println("🔄 FSM: → idle")

	case bot.StateEditingDate:
		start, days, err := calc.ParseTaskInput(text)
		if err != nil {
			fmt.Printf("❌ Ошибка: %v\n", err)
			return
		}
		task, _ := repo.GetByID(ctx, state.TaskID)
		task.StartDate = start
		task.DurationDays = days
		if err := repo.Update(ctx, task); err != nil {
			fmt.Printf("❌ Ошибка обновления: %v\n", err)
			return
		}
		fsm.Reset(TestTGID)
		fmt.Println("✅ Дата обновлена в БД")
		fmt.Println("🔄 FSM: → idle")

	default:
		fmt.Printf("⚠️ Текстовый ввод проигнорирован в состоянии: %s\n", state.State)
	}
}

func runListTasks(ctx context.Context, repo *repository.SQLiteRepo) {
	user, _ := repo.GetUserByTGID(ctx, TestTGID)
	tasks, err := repo.GetByUserID(ctx, user.ID)
	if err != nil {
		fmt.Printf("❌ Ошибка загрузки: %v\n", err)
		return
	}
	if len(tasks) == 0 {
		fmt.Println("📭 Задач нет")
		return
	}
	fmt.Println("📋 Задачи из БД:")
	for _, t := range tasks {
		end := t.StartDate.AddDate(0, 0, t.DurationDays-1)
		fmt.Printf("   #%d | %q | %s → %s | Важность: %d\n",
			t.ID, t.Name, t.StartDate.Format("02.01"), end.Format("02.01"), t.Importance)
	}
}

func runEditStart(ctx context.Context, fsm *bot.FSM, repo *repository.SQLiteRepo, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		fmt.Println("❌ Ожидался числовой ID")
		return
	}
	task, err := repo.GetByID(ctx, id)
	if err != nil || task == nil {
		fmt.Printf("❌ Задача #%d не найдена\n", id)
		return
	}
	// В реальности тут был бы выбор поля, для теста сразу переходим к имени
	fsm.Set(TestTGID, bot.FSMState{State: bot.StateEditingName, TaskID: task.ID})
	fmt.Printf("🔄 FSM: → editing:name (task=%d, текущее имя: %q)\n", task.ID, task.Name)
	fmt.Println("💬 Ожидание: введите новое название")
}

func runVerify(ctx context.Context, repo *repository.SQLiteRepo, fsm *bot.FSM) {
	fmt.Println("\n🔍 === ГЛУБОКАЯ ПРОВЕРКА ===")
	user, _ := repo.GetUserByTGID(ctx, TestTGID)
	fmt.Printf("👤 Юзер: ID=%d, TG_ID=%d, Username=%q\n", user.ID, user.TG_ID, user.Username)

	tasks, _ := repo.GetByUserID(ctx, user.ID)
	fmt.Printf("📦 Всего задач: %d\n", len(tasks))

	state, ok := fsm.Get(TestTGID)
	if ok {
		fmt.Printf("📍 FSM: state=%s, taskID=%d, payload=%q\n", state.State, state.TaskID, state.Payload)
	} else {
		fmt.Println("📍 FSM: idle (нет активной сессии)")
	}
	fmt.Println("✅ Проверка завершена")
}

func printFSMState(fsm *bot.FSM) {
	if state, ok := fsm.Get(TestTGID); ok {
		fmt.Printf("📍 [FSM] %s (task=%d) | ", state.State, state.TaskID)
	} else {
		fmt.Print("📍 [FSM] idle | ")
	}
}

func cleanupTestDB(db *sql.DB) {
	// Удаляем тестового юзера и его задачи, чтобы тесты не накапливались
	db.Exec("DELETE FROM notifications WHERE user_id = (SELECT id FROM users WHERE tg_id = ?)", TestTGID)
	db.Exec("DELETE FROM tasks WHERE user_id = (SELECT id FROM users WHERE tg_id = ?)", TestTGID)
	db.Exec("DELETE FROM users WHERE tg_id = ?", TestTGID)
}