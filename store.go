package main

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Note — обычная заметка без времени.
type Note struct {
	ID        int64  `json:"id"`
	Text      string `json:"text"`
	Tags      string `json:"tags"`
	Pinned    bool   `json:"pinned"`
	CreatedAt int64  `json:"created_at"`
}

// Reminder — заметка с моментом, когда нужно напомнить.
type Reminder struct {
	ID        int64  `json:"id"`
	Text      string `json:"text"`
	DueAt     int64  `json:"due_at"`
	Done      bool   `json:"done"`
	Fired     bool   `json:"fired"`
	CreatedAt int64  `json:"created_at"`
}

const schemaSQL = `
create table if not exists notes (
	id integer primary key autoincrement,
	text text not null,
	tags text not null default '',
	pinned integer not null default 0,
	created_at integer not null
);

create table if not exists reminders (
	id integer primary key autoincrement,
	text text not null,
	due_at integer not null,
	done integer not null default 0,
	fired integer not null default 0,
	created_at integer not null
);

create index if not exists notes_created on notes (created_at desc);
create index if not exists reminders_due on reminders (due_at);
`

// Store — встроенная база (SQLite) в каталоге приложения.
type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---------- заметки ----------

func extractTags(text string) string {
	var tags []string
	for _, f := range strings.Fields(text) {
		if strings.HasPrefix(f, "#") && len([]rune(f)) > 1 {
			tags = append(tags, f)
		}
	}
	return strings.Join(tags, " ")
}

func (s *Store) Notes(search string) ([]Note, error) {
	where := "1 = 1"
	args := []any{}
	if search = strings.TrimSpace(search); search != "" {
		where = "(text like ? or tags like ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}

	rows, err := s.db.Query(`
		select id, text, tags, pinned, created_at
		from notes where `+where+`
		order by pinned desc, created_at desc limit 1000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Note{}
	for rows.Next() {
		var n Note
		var pinned int
		if err := rows.Scan(&n.ID, &n.Text, &n.Tags, &pinned, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Pinned = pinned == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) CreateNote(text string) (Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Note{}, errors.New("пустая заметка")
	}
	now := time.Now().Unix()
	tags := extractTags(text)
	res, err := s.db.Exec(`insert into notes (text, tags, created_at) values (?, ?, ?)`,
		text, tags, now)
	if err != nil {
		return Note{}, err
	}
	id, _ := res.LastInsertId()
	return Note{ID: id, Text: text, Tags: tags, CreatedAt: now}, nil
}

func (s *Store) SetNotePinned(id int64, pinned bool) error {
	value := 0
	if pinned {
		value = 1
	}
	res, err := s.db.Exec(`update notes set pinned = ? where id = ?`, value, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) DeleteNote(id int64) error {
	res, err := s.db.Exec(`delete from notes where id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// ---------- напоминания ----------

func (s *Store) Reminders() ([]Reminder, error) {
	rows, err := s.db.Query(`
		select id, text, due_at, done, fired, created_at
		from reminders
		order by done asc, due_at asc limit 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Reminder{}
	for rows.Next() {
		var r Reminder
		var done, fired int
		if err := rows.Scan(&r.ID, &r.Text, &r.DueAt, &done, &fired, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Done = done == 1
		r.Fired = fired == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateReminder(text string, dueAt int64) (Reminder, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Reminder{}, errors.New("пустое напоминание")
	}
	if dueAt <= 0 {
		return Reminder{}, errors.New("не задано время напоминания")
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(`insert into reminders (text, due_at, created_at) values (?, ?, ?)`,
		text, dueAt, now)
	if err != nil {
		return Reminder{}, err
	}
	id, _ := res.LastInsertId()
	return Reminder{ID: id, Text: text, DueAt: dueAt, CreatedAt: now}, nil
}

func (s *Store) SetReminderDone(id int64, done bool) error {
	value, fired := 0, 0
	if done {
		value, fired = 1, 1
	}
	res, err := s.db.Exec(`update reminders set done = ?, fired = ? where id = ?`, value, fired, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) DeleteReminder(id int64) error {
	res, err := s.db.Exec(`delete from reminders where id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// DueReminders — те, время которых наступило, а напоминание ещё не сработало.
func (s *Store) DueReminders(now int64) ([]Reminder, error) {
	rows, err := s.db.Query(`
		select id, text, due_at, done, fired, created_at
		from reminders
		where done = 0 and fired = 0 and due_at <= ?
		order by due_at asc limit 50`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Reminder{}
	for rows.Next() {
		var r Reminder
		var done, fired int
		if err := rows.Scan(&r.ID, &r.Text, &r.DueAt, &done, &fired, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Done = done == 1
		r.Fired = fired == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MarkRemindersFired(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	_, err := s.db.Exec(`update reminders set fired = 1 where id in (`+
		strings.Join(placeholders, ",")+`)`, args...)
	return err
}

var errNotFound = errors.New("не найдено")
