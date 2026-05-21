package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"tg_sheduler/internal/domain"
	"tg_sheduler/internal/repository"

	tele "gopkg.in/telebot.v4"
)

type Handlers struct {
	taskRepo  repository.TaskRepository
	notifRepo repository.NotificationRepository
	userRepo  repository.UserRepository
	fsm       *FSM
	calc      *domain.TaskCalculator
	location  *time.Location
}

func NewHandlers(
	taskRepo repository.TaskRepository,
	notifRepo repository.NotificationRepository,
	userRepo repository.UserRepository,
	fsm *FSM,
	calc *domain.TaskCalculator,
	loc *time.Location,
) *Handlers {
	return &Handlers{
		taskRepo:  taskRepo,
		notifRepo: notifRepo,
		userRepo:  userRepo,
		fsm:       fsm,
		calc:      calc,
		location:  loc,
	}
}

// ensureUser — создаёт пользователя, если нет
func (h *Handlers) ensureUser(ctx context.Context, tgID int, username string) (*domain.User, error) {
	user, err := h.userRepo.GetUserByTGID(ctx, tgID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return h.userRepo.CreateUser(ctx, &domain.User{
			TG_ID:    tgID,
			Username: username,
		})
	}
	return user, nil
}

// Start — команда /start
func (h *Handlers) Start(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.fsm.Reset(c.Sender().ID)

	// Авто-регистрация
	_, _ = h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)

	markup := &tele.ReplyMarkup{}
	btnCreate := markup.Data("✨ Создать задачу", "create_task")
	btnMyTasks := markup.Data("📋 Мои задачи", "my_tasks")
	markup.Inline(markup.Row(btnCreate, btnMyTasks))

	return c.Send("👋 Привет! Я твой персональный планировщик задач.\nЧто будем делать?", markup)
}

// Cancel — отмена текущего действия
func (h *Handlers) Cancel(c tele.Context) error {
	h.fsm.Reset(c.Sender().ID)

	markup := &tele.ReplyMarkup{}
	btnCreate := markup.Data("✨ Создать задачу", "create_task")
	btnMyTasks := markup.Data("📋 Мои задачи", "my_tasks")
	markup.Inline(markup.Row(btnCreate, btnMyTasks))

	return c.Send("✅ Главное меню:", markup)
}

// Callback — обработка inline-кнопок
func (h *Handlers) Callback(c tele.Context) error {
	data := c.Data()
	parts := strings.SplitN(data, ":", 2)
	action := parts[0]
	payload := ""
	if len(parts) > 1 {
		payload = parts[1]
	}

	switch action {
	case "\fcreate_task":
		h.fsm.Set(c.Sender().ID, FSMState{State: StateCreatingName})
		return c.Send("✍️ *Введите название задачи:*\n(или /cancel для отмены)", tele.ModeMarkdown)

	case "\fmy_tasks":
		return h.listTasks(c)

	case "\fedit":
		taskID, err := strconv.Atoi(payload)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка ID задачи", ShowAlert: true})
		}
		return h.showEditOptions(c, taskID)

	case "\fedit_name":
		taskID, _ := strconv.Atoi(payload)
		h.fsm.Set(c.Sender().ID, FSMState{State: StateEditingName, TaskID: taskID})
		return c.Send("✏️ Введите *новое название* задачи:\n(или /cancel)", tele.ModeMarkdown)

	case "\fedit_date":
		taskID, _ := strconv.Atoi(payload)
		h.fsm.Set(c.Sender().ID, FSMState{State: StateEditingDate, TaskID: taskID})
		return c.Send("📅 Введите дату в формате:\n`19.05 5`\nдата начала пробел продолжительность в днях\n(или /cancel)", tele.ModeMarkdown, tele.NoPreview)

	case "\fedit_importance":
		taskID, _ := strconv.Atoi(payload)
		return h.showImportanceButtons(c, taskID)

	case "\fimp_create":
		// Создание задачи с выбранной важностью
		imp, err := strconv.Atoi(payload)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка", ShowAlert: true})
		}
		return h.handleCreateWithImportance(c, imp)

	case "\fimp":
		// payload: "level:taskID"
		sub := strings.Split(payload, ":")
		if len(sub) != 2 {
			return c.Respond(&tele.CallbackResponse{Text: "❌ Неверный формат", ShowAlert: true})
		}
		imp, _ := strconv.Atoi(sub[0])
		taskID, _ := strconv.Atoi(sub[1])
		return h.updateImportance(c, taskID, domain.Importance(imp))

	case "\fback":
		h.fsm.Reset(c.Sender().ID)
		return h.Start(c)

	default:
		return c.Respond(&tele.CallbackResponse{Text: "🚧 Пока в разработке", ShowAlert: false})
	}
}

// HandleText — обработка текстовых сообщений (FSM)
func (h *Handlers) HandleText(c tele.Context) error {
	userID := c.Sender().ID
	text := strings.TrimSpace(c.Text())

	// Игнорируем команды в тексте
	if strings.HasPrefix(text, "/") {
		return nil
	}

	if text == "Вернуться к выбору" {
		h.Cancel(c)
		return nil
	}

	state, ok := h.fsm.Get(userID)
	if !ok {
		return c.Send("🤔 Не понял команду. Нажми /start или выбери кнопку.")
	}

	switch state.State {
	// === Создание задачи ===
	case StateCreatingName:
		return h.handleCreatingName(c, text)
	case StateCreatingDate:
		return h.handleCreatingDate(c, text)
	case StateCreatingImportance:
		return h.handleCreatingImportance(c, text)

	// === Редактирование задачи ===
	case StateEditingName:
		return h.handleEditingName(c, text)
	case StateEditingDate:
		return h.handleEditingDate(c, text)
	// case StateEditingImportance:
	// 	return h.handleEditingImportance(c, text)

	default:
		return c.Send("🤷 Не знаю, что делать с этим текстом. Нажми /cancel или выбери кнопку.")
	}
}

// --- Создание задачи: шаг 1 (название) ---
func (h *Handlers) handleCreatingName(c tele.Context, name string) error {
	if name == "" {
		return c.Send("❌ Название не может быть пустым. Попробуй ещё раз:")
	}

	// Сохраняем название во временное состояние
	h.fsm.Set(c.Sender().ID, FSMState{
		State:   StateCreatingDate,
		Payload: name, // временно храним название здесь
	})

	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(markup.Data("🔙 Назад", "back")))

	return c.Send("📅 Теперь введите *дату и длительность* в формате:\n`19.05 5`\n_19 мая, задача на 5 дней_\n(или нажми 🔙 Назад)", tele.ModeMarkdown, tele.NoPreview, markup)
}

// --- Создание задачи: шаг 2 (дата) ---
func (h *Handlers) handleCreatingDate(c tele.Context, input string) error {
	// Проверка на "Назад" через текст (если пользователь не хочет использовать кнопку)
	if strings.ToLower(input) == "назад" || strings.ToLower(input) == "back" {
		h.fsm.Set(c.Sender().ID, FSMState{State: StateCreatingName})
		return c.Send("✍️ Введите название задачи:")
	}

	// Парсим дату
	start, days, err := h.calc.ParseTaskInput(input)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ %v\nПопробуй ещё раз в формате `19.05 5`:", err), tele.ModeMarkdown)
	}

	// Получаем название из предыдущего шага (храним в Payload предыдущего FSMState)
	prev, _ := h.fsm.Get(c.Sender().ID)
	name := prev.Payload // это название из шага 1

	// Сохраняем промежуточные данные
	h.fsm.Set(c.Sender().ID, FSMState{
		State:   StateCreatingImportance,
		Payload: fmt.Sprintf("%s|%s|%d", name, start.Format(time.RFC3339), days), // временно: название|оригинальный ввод|дни
		// название берём из предыдущего состояния
	})



	// Кнопки важности
	markup := &tele.ReplyMarkup{}
	btn1 := markup.Data("1️⃣ Не важная", "imp_create:0")
	btn2 := markup.Data("2️⃣ Важная", "imp_create:1")
	btn3 := markup.Data("3️⃣ Очень важная", "imp_create:2")
	markup.Inline(
		markup.Row(btn1, btn2, btn3),
		markup.Row(markup.Data("🔙 Назад", "back")),
	)

	// // Сохраняем данные задачи во временное хранилище (в самом FSM или можно в кэш)
	// // Для простоты — кодируем в Payload: "name|startRFC3339|days"
	// payload := fmt.Sprintf("%s|%s|%d", name, start.Format(time.RFC3339), days)
	// h.fsm.Set(c.Sender().ID, FSMState{
	// 	State:   StateCreatingImportance,
	// 	Payload: payload,
	// })

	return c.Send("🎯 Выбери *важность* задачи:", tele.ModeMarkdown, markup)
}

// --- Создание задачи: шаг 3 (важность) + сохранение ---
func (h *Handlers) handleCreatingImportance(c tele.Context, input string) error {
	// Этот метод вызывается ТОЛЬКО если пользователь ввёл текст, а не нажал кнопку.
	// Кнопки обрабатываются в Callback через imp_create:...
	return c.Send("👆 Пожалуйста, выбери важность *кнопкой* ниже:", tele.ModeMarkdown)
}

// Обработчик выбора важности ПРИ СОЗДАНИИ (через callback)
func (h *Handlers) handleCreateWithImportance(c tele.Context, impLevel int) error {
	state, ok := h.fsm.Get(c.Sender().ID)
	if !ok || state.State != StateCreatingImportance {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Сессия истекла", ShowAlert: true})
	}

	// Десериализуем payload: "name|startRFC3339|days"
	parts := strings.SplitN(state.Payload, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка данных", ShowAlert: true})
	}
	name := parts[0]
	start, _ := time.Parse(time.RFC3339, parts[1])
	days, _ := strconv.Atoi(parts[2])

	// Создаём задачу
	ctx := context.Background()
	user, err := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка пользователя: " + err.Error())
	}

	task := &domain.Task{
		UserID:       user.ID,
		Name:         name,
		Importance:   int(impLevel),
		StartDate:    start,
		DurationDays: days,
	}

	if err := h.taskRepo.Create(ctx, task); err != nil {
		return c.Send("❌ Не удалось сохранить задачу: " + err.Error())
	}

	// Генерируем уведомления
	notifications := h.calc.CalculateNotifications(start, days, domain.Importance(impLevel))
	for i := range notifications {
		notifications[i].TaskID = task.ID
		notifications[i].UserID = user.ID
	}
	if err := h.notifRepo.CreateBatch(ctx, notifications); err != nil {
		log.Printf("⚠️ Failed to save notifications: %v", err)
		// Не прерываем, задача уже создана
	}

	// Успех!
	h.fsm.Reset(c.Sender().ID)
	return c.Send(fmt.Sprintf("✅ Задача *\"%s\"* создана!\n🔔 Будет %d оповещений.", name, len(notifications)), tele.ModeMarkdown)
}

// --- Список задач ---
func (h *Handlers) listTasks(c tele.Context) error {
	ctx := context.Background()
	user, err := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)
	if err != nil {
		return c.Send("❌ Ошибка: " + err.Error())
	}

	tasks, err := h.taskRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		return c.Send("❌ Не удалось загрузить задачи")
	}

	if len(tasks) == 0 {
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(markup.Data("✨ Создать первую", "create_task")))
		return c.Send("📭 У тебя пока нет задач. Давай создадим первую!", markup)
	}

	var text strings.Builder
	text.WriteString("📋 *Твои задачи:*\n\n")
	for i, t := range tasks {
		num := i + 1
		endDate := t.StartDate.AddDate(0, 0, t.DurationDays-1)
		text.WriteString(fmt.Sprintf("%d. *%s*\n   📅 %s — %s (%d дн.)\n   🔖 Важность: %s\n\n",
			num, t.Name,
			t.StartDate.Format("02.01"), endDate.Format("02.01"),
			t.DurationDays,
			importanceLabel(domain.Importance(t.Importance)),
		))
	}

	// Кнопки с номерами задач
	markup := &tele.ReplyMarkup{}
	var row []tele.Btn
	for i, t := range tasks {
		row = append(row, markup.Data(fmt.Sprintf("#%d", i+1), fmt.Sprintf("edit:%d", t.ID)))
		if len(row) == 3 {
			markup.Inline(markup.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		markup.Inline(markup.Row(row...))
	}
	// markup.Inline(markup.Row(markup.Data("🔙 Назад", "back")))

	return c.Send(text.String(), markup, tele.ModeMarkdown)
}

// --- Редактирование: показать опции ---
func (h *Handlers) showEditOptions(c tele.Context, taskID int) error {
	ctx := context.Background()
	task, err := h.taskRepo.GetByID(ctx, taskID)
	if err != nil || task == nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Задача не найдена", ShowAlert: true})
	}

	// Проверка: задача принадлежит пользователю
	user, _ := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)
	if task.UserID != user.ID {
		return c.Respond(&tele.CallbackResponse{Text: "🔒 Не твоя задача", ShowAlert: true})
	}

	markup := &tele.ReplyMarkup{}
	btnName := markup.Data("✏️ Название", fmt.Sprintf("edit_name:%d", taskID))
	btnDate := markup.Data("📅 Дату", fmt.Sprintf("edit_date:%d", taskID))
	btnImp := markup.Data("🔖 Важность", fmt.Sprintf("edit_importance:%d", taskID))
	btnBack := markup.Data("🔙 Назад", "back")

	// Сохраняем состояние редактирования
	h.fsm.Set(c.Sender().ID, FSMState{State: StateEditingSelect, TaskID: taskID})

	markup.Inline(
		markup.Row(btnName, btnDate, btnImp),
		markup.Row(btnBack),
	)

	endDate := task.StartDate.AddDate(0, 0, task.DurationDays-1)
	text := fmt.Sprintf("✏️ *Редактирование задачи #%d*\n📌 %s\n📅 %s — %s\n🔖 %s\n\nЧто изменить?",
		task.ID, task.Name,
		task.StartDate.Format("02.01"), endDate.Format("02.01"),
		importanceLabel(domain.Importance(task.Importance)),
	)

	return c.Send(text, markup, tele.ModeMarkdown)
}

// --- Редактирование: название ---
func (h *Handlers) handleEditingName(c tele.Context, newName string) error {
	state, _ := h.fsm.Get(c.Sender().ID)
	ctx := context.Background()

	task, err := h.taskRepo.GetByID(ctx, state.TaskID)
	if err != nil || task == nil {
		h.fsm.Reset(c.Sender().ID)
		return c.Send("❌ Задача не найдена")
	}

	// Проверка прав
	user, _ := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)
	if task.UserID != user.ID {
		return c.Send("🔒 Нельзя редактировать чужие задачи")
	}

	task.Name = newName
	if err := h.taskRepo.Update(ctx, task); err != nil {
		return c.Send("❌ Ошибка обновления: " + err.Error())
	}

	h.fsm.Reset(c.Sender().ID)
	return c.Send(fmt.Sprintf("✅ Название изменено на *\"%s\"*", newName), tele.ModeMarkdown)
}

// --- Редактирование: дата ---
func (h *Handlers) handleEditingDate(c tele.Context, input string) error {
	state, _ := h.fsm.Get(c.Sender().ID)
	ctx := context.Background()

	task, err := h.taskRepo.GetByID(ctx, state.TaskID)
	if err != nil || task == nil {
		h.fsm.Reset(c.Sender().ID)
		return c.Send("❌ Задача не найдена")
	}

	// Проверка прав
	user, _ := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)
	if task.UserID != user.ID {
		return c.Send("🔒 Нельзя редактировать чужие задачи")
	}

	// Парсим новую дату
	start, days, err := h.calc.ParseTaskInput(input)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ %v\nПопробуй ещё раз:", err))
	}

	// Обновляем
	task.StartDate = start
	task.DurationDays = days
	if err := h.taskRepo.Update(ctx, task); err != nil {
		return c.Send("❌ Ошибка: " + err.Error())
	}

	// Пересоздаём уведомления
	_ = h.notifRepo.DeleteByTaskID(ctx, task.ID)
	notifs := h.calc.CalculateNotifications(start, days, domain.Importance(task.Importance))
	for i := range notifs {
		notifs[i].TaskID = task.ID
		notifs[i].UserID = user.ID
	}
	_ = h.notifRepo.CreateBatch(ctx, notifs)

	h.fsm.Reset(c.Sender().ID)
	return c.Send(fmt.Sprintf("✅ Дата обновлена!\n🔔 Сгенерировано %d новых оповещений.", len(notifs)))
}

// --- Показать кнопки важности для редактирования ---
func (h *Handlers) showImportanceButtons(c tele.Context, taskID int) error {
	markup := &tele.ReplyMarkup{}
	var row []tele.Btn
	for i := 1; i <= 3; i++ {
		label := map[int]string{1: "1️⃣ Не важная", 2: "2️⃣ Важная", 3: "3️⃣ Очень важная"}[i]
		row = append(row, markup.Data(label, fmt.Sprintf("imp:%d:%d", i-1, taskID)))
	}
	markup.Inline(markup.Row(row...))
	//markup.Inline(markup.Row(markup.Data("🔙 Назад", fmt.Sprintf("edit:%d", taskID))))

	return c.Send("🔖 Выбери новую важность:", markup)
}

// --- Обновление важности (и при создании, и при редактировании) ---
func (h *Handlers) updateImportance(c tele.Context, taskID int, newImp domain.Importance) error {
	// Проверяем: это создание или редактирование?
	state, ok := h.fsm.Get(c.Sender().ID)

	ctx := context.Background()
	user, _ := h.ensureUser(ctx, int(c.Sender().ID), c.Sender().Username)

	if !ok || state.State == StateCreatingImportance {
		// === СОЗДАНИЕ ===
		// Обработаем создание с выбранной важностью
		// (эта логика дублируется, но для простоты оставим так)
		// В реальном проекте лучше вынести в отдельный метод
		// Для краткости — вызываем уже написанный хендлер через "костыль":
		// Но правильнее — рефакторить. Пока просто редиректим:
		_ = c // заглушка, чтобы не было warning
		// Реальная логика создания уже в handleCreateWithImportance,
		// который вызывается из Callback для imp_create:...
		return nil
	}

	// === РЕДАКТИРОВАНИЕ ===
	task, err := h.taskRepo.GetByID(ctx, taskID)
	if err != nil || task == nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Задача не найдена", ShowAlert: true})
	}
	if task.UserID != user.ID {
		return c.Respond(&tele.CallbackResponse{Text: "🔒 Не твоя задача", ShowAlert: true})
	}

	task.Importance = int(newImp)
	if err := h.taskRepo.Update(ctx, task); err != nil {
		return c.Send("❌ Ошибка: " + err.Error())
	}

	// Пересоздаём уведомления
	_ = h.notifRepo.DeleteByTaskID(ctx, task.ID)
	notifs := h.calc.CalculateNotifications(task.StartDate, task.DurationDays, newImp)
	for i := range notifs {
		notifs[i].TaskID = task.ID
		notifs[i].UserID = user.ID
	}
	_ = h.notifRepo.CreateBatch(ctx, notifs)

	h.fsm.Reset(c.Sender().ID)
	return c.Send(fmt.Sprintf("✅ Важность изменена на *%s*\n🔔 Оповещений: %d", importanceLabel(newImp), len(notifs)), tele.ModeMarkdown)
}

// --- Хелпер: текстовое представление важности ---
func importanceLabel(imp domain.Importance) string {
	switch imp {
	case domain.Low:
		return "Не важная"
	case domain.Medium:
		return "Важная"
	case domain.High:
		return "Очень важная"
	default:
		return "Неизвестно"
	}
}
