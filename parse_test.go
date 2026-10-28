package main

import (
	"testing"
	"time"
)

func TestParseCaptureNote(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)

	cases := []struct {
		in   string
		text string
	}{
		{"купить молоко", "купить молоко"},
		{"  #работа отправить документы  ", "#работа отправить документы"},
		{"созвон в 14:00 с Петей", "созвон в 14:00 с Петей"},
	}

	for _, c := range cases {
		got := ParseCapture(c.in, now)
		if got.Kind != "note" {
			t.Fatalf("%q: ожидалась заметка, получено %q (%s)", c.in, got.Kind, got.Error)
		}
		if got.Text != c.text {
			t.Fatalf("%q: текст %q, ожидался %q", c.in, got.Text, c.text)
		}
	}
}

func TestParseCaptureReminder(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)

	cases := []struct {
		in   string
		text string
		want time.Time
	}{
		// время ещё не наступило — сегодня
		{"@14:00 позвонить в банк", "позвонить в банк",
			time.Date(2026, 10, 28, 14, 0, 0, 0, time.Local)},
		// время прошло — переносится на завтра
		{"@9 зарядка", "зарядка",
			time.Date(2026, 10, 29, 9, 0, 0, 0, time.Local)},
		// явное «завтра»
		{"@14:30 завтра встреча", "встреча",
			time.Date(2026, 10, 29, 14, 30, 0, 0, time.Local)},
		{"@завтра 8:15 созвон", "созвон",
			time.Date(2026, 10, 29, 8, 15, 0, 0, time.Local)},
		// дата
		{"@28.10 18:00 сдать отчёт", "сдать отчёт",
			time.Date(2026, 10, 28, 18, 0, 0, 0, time.Local)},
		{"@05.11 09:30 врач", "врач",
			time.Date(2026, 11, 5, 9, 30, 0, 0, time.Local)},
		// время + «завтра»
		{"@16:00 завтра забрать посылку", "забрать посылку",
			time.Date(2026, 10, 29, 16, 0, 0, 0, time.Local)},
		// относительное
		{"@через 30 мин выключить чайник", "выключить чайник",
			now.Add(30 * time.Minute)},
	}

	for _, c := range cases {
		got := ParseCapture(c.in, now)
		if got.Kind != "reminder" {
			t.Fatalf("%q: ожидалось напоминание, получено %q (%s)", c.in, got.Kind, got.Error)
		}
		if got.Text != c.text {
			t.Fatalf("%q: текст %q, ожидался %q", c.in, got.Text, c.text)
		}
		if got.DueAt != c.want.Unix() {
			t.Fatalf("%q: время %s, ожидалось %s",
				c.in,
				time.Unix(got.DueAt, 0).Format("02.01 15:04"),
				c.want.Format("02.01 15:04"))
		}
		if got.DueLabel == "" {
			t.Fatalf("%q: пустая подпись времени", c.in)
		}
	}
}

func TestParseCaptureErrors(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)

	for _, in := range []string{"", "   ", "@", "@абракадабра текст", "@14:00"} {
		got := ParseCapture(in, now)
		if got.Kind != "error" && got.Kind != "note" {
			t.Fatalf("%q: неожиданный вид %q", in, got.Kind)
		}
		if in == "@" || in == "@абракадабра текст" || in == "@14:00" {
			if got.Kind != "error" {
				t.Fatalf("%q: ожидалась ошибка разбора, получено %q", in, got.Kind)
			}
		}
	}
}

func TestParseCapturePastTimeRollsToTomorrow(t *testing.T) {
	now := time.Date(2026, 10, 28, 23, 30, 0, 0, time.Local)
	got := ParseCapture("@22:00 проверить", now)
	if got.Kind != "reminder" {
		t.Fatalf("ожидалось напоминание, получено %q", got.Kind)
	}
	want := time.Date(2026, 10, 29, 22, 0, 0, 0, time.Local)
	if got.DueAt != want.Unix() {
		t.Fatalf("время %s, ожидалось %s",
			time.Unix(got.DueAt, 0).Format("02.01 15:04"), want.Format("02.01 15:04"))
	}
}
