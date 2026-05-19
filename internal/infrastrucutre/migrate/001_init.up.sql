-- users: привязка Telegram ID к нашей системе
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tg_id INTEGER UNIQUE NOT NULL,
    username TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- tasks: сами задачи
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    importance INTEGER NOT NULL, -- 0=Low, 1=Medium, 2=High
    start_date TEXT NOT NULL,        
    duration_days INTEGER NOT NULL, 
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_tasks_user ON tasks(user_id);

-- notifications: рассчитанные точки срабатывания
CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    time TEXT NOT NULL,        -- RFC3339 формат: "2026-01-11T12:00:00+03:00"
    label TEXT,
    is_sent BOOLEAN DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_notifications_pending ON notifications(is_sent, time);