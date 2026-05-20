package repository

import (
	"context"
	"database/sql"
	"fmt"
	"tg_sheduler/internal/domain"
	"time"
)

type SQLiteRepo struct {
	db *sql.DB
}

func NewSQLiteRepo(db *sql.DB) *SQLiteRepo {
	return &SQLiteRepo{db: db}
}

// ============ UserRepository ============

func (r *SQLiteRepo) CreateUser(ctx context.Context, user *domain.User) (*domain.User, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO users (tg_id, username) VALUES (?, ?) ON CONFLICT(tg_id) DO UPDATE SET username = excluded.username",
		user.TG_ID, user.Username,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	id, _ := res.LastInsertId()
	user.ID = int(id)
	return user, nil
}

func (r *SQLiteRepo) GetUserByTGID(ctx context.Context, id int) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, tg_id, username FROM users WHERE tg_id = ?", id)

	var u domain.User
	err := row.Scan(&u.ID, &u.TG_ID, &u.Username)
	if err == sql.ErrNoRows {
		return nil, nil // не ошибка, просто не найден
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (r *SQLiteRepo) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, tg_id, username FROM users WHERE id = ?", id)

	var u domain.User
	err := row.Scan(&u.ID, &u.TG_ID, &u.Username)
	if err == sql.ErrNoRows {
		return nil, nil // не ошибка, просто не найден
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

// ============ TaskRepository ============

func (r *SQLiteRepo) Create(ctx context.Context, task *domain.Task) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tasks (user_id, name, importance, start_date, duration_days) 
         VALUES (?, ?, ?, ?, ?)`,
		task.UserID,
		task.Name,
		task.Importance,
		task.StartDate.Format("2006-01-02"),
		task.DurationDays,
	)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}

	id, _ := res.LastInsertId()
	task.ID = int(id)
	return nil
}

func (r *SQLiteRepo) GetByID(ctx context.Context, id int) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, user_id, name, start_date, duration_days, importance FROM tasks WHERE id = ?", id)

	var t domain.Task
	var startDateStr string
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &startDateStr, &t.DurationDays, &t.Importance)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	t.StartDate, _ = time.Parse("2006-01-02", startDateStr)
	return &t, nil
}

func (r *SQLiteRepo) GetByUserID(ctx context.Context, userID int) ([]*domain.Task, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, user_id, name, importance, start_date, duration_days FROM tasks WHERE user_id = ? ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		var t domain.Task
		var startDateStr string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Importance, &startDateStr, &t.DurationDays); err != nil {
			return nil, err
		}
		t.StartDate, err = time.Parse("2006-01-02", startDateStr)
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (r *SQLiteRepo) Update(ctx context.Context, task *domain.Task) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE tasks SET name = ?, importance = ?, start_date = ?, duration_days = ? WHERE id = ?",
		task.Name, task.Importance, task.StartDate, task.DurationDays, task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (r *SQLiteRepo) Delete(ctx context.Context, id int) error {
	// notifications удалятся автоматически благодаря ON DELETE CASCADE
	_, err := r.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// ============ NotificationRepository ============

func (r *SQLiteRepo) CreateBatch(ctx context.Context, notifs []domain.Notification) error {
	if len(notifs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // откат, если не закоммитим

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO notifications (task_id, user_id, time, label, is_sent) VALUES (?, ?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, n := range notifs {
		_, err := stmt.ExecContext(ctx,
			n.TaskID,
			n.UserID,
			n.Time.Format(time.RFC3339), // храним как строку в надёжном формате
			n.Label,
			n.IsSent,
		)
		if err != nil {
			return fmt.Errorf("insert notif: %w", err)
		}
	}

	return tx.Commit()
}

func (r *SQLiteRepo) GetNotifications(ctx context.Context) ([]*domain.Notification, error) {
	now := time.Now().Format(time.RFC3339)

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_id, user_id, time, label, is_sent 
		 FROM notifications 
		 WHERE is_sent = 0 AND time <= ? 
		 ORDER BY time ASC `,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending: %w", err)
	}
	defer rows.Close()

	var notifs []*domain.Notification
	for rows.Next() {
		var n domain.Notification
		var timeStr string
		if err := rows.Scan(&n.ID, &n.TaskID, &n.UserID, &timeStr, &n.Label, &n.IsSent); err != nil {
			return nil, err
		}
		n.Time, _ = time.Parse(time.RFC3339, timeStr) // игнорируем ошибку, т.к. сами же записывали
		notifs = append(notifs, &n)
	}
	return notifs, rows.Err()
}

func (r *SQLiteRepo) MarkAsSent(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE notifications SET is_sent = 1 WHERE id = ? AND is_sent = 0", id)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

func (r *SQLiteRepo) DeleteByTaskID(ctx context.Context, taskID int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM notifications WHERE task_id = ?", taskID)
	if err != nil {
		return fmt.Errorf("delete notifs: %w", err)
	}
	return nil
}
