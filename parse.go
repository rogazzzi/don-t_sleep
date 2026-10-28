package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CaptureResult — результат разбора быстрой записи.
// Kind: "note" | "reminder" | "error".
type CaptureResult struct {
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	DueAt    int64  `json:"due_at,omitempty"`
	DueLabel string `json:"due_label,omitempty"`
	Error    string `json:"error,omitempty"`
}

var (
	reHHMM = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	reHour = regexp.MustCompile(`^(\d{1,2})$`)
	reDate = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})$`)
)

// ParseCapture решает, что создаёт пользователь: заметку или напоминание.
// Правило простое и предсказуемое: есть ведущий @ — напоминание, иначе заметка.
//
//	@14:00 позвонить в банк        → сегодня в 14:00
//	@9 купить молоко               → сегодня/завтра в 09:00
//	@14:30 завтра встреча          → завтра в 14:30
//	@28.10 09:00 сдать отчёт       → 28 октября в 09:00
//	@через 30 мин забрать посылку  → через 30 минут
func ParseCapture(raw string, now time.Time) CaptureResult {
	text := strings.TrimSpace(raw)
	if text == "" {
		return CaptureResult{Kind: "error", Error: "пустой ввод"}
	}
	if strings.HasPrefix(text, "@") {
		return parseReminder(strings.TrimSpace(text[1:]), now)
	}
	return CaptureResult{Kind: "note", Text: text}
}

func parseReminder(s string, now time.Time) CaptureResult {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return CaptureResult{Kind: "error", Error: "после @ нужно время, например @14:00"}
	}

	due, consumed, label, err := parseWhen(fields, now)
	if err != nil {
		return CaptureResult{Kind: "error", Error: err.Error()}
	}

	text := strings.TrimSpace(strings.Join(fields[consumed:], " "))
	if text == "" {
		return CaptureResult{Kind: "error", Error: "нужен текст напоминания после времени"}
	}

	return CaptureResult{Kind: "reminder", Text: text, DueAt: due.Unix(), DueLabel: label}
}

func parseWhen(fields []string, now time.Time) (time.Time, int, string, error) {
	loc := now.Location()
	at := func(y int, mo time.Month, d, h, mi int) time.Time {
		return time.Date(y, mo, d, h, mi, 0, 0, loc)
	}

	first := fields[0]

	// @завтра [HH:MM | HH]
	if first == "завтра" {
		base := at(now.Year(), now.Month(), now.Day(), 9, 0, 0).AddDate(0, 0, 1)
		consumed := 1
		if len(fields) > 1 {
			if h, mi, ok := parseHHMM(fields[1]); ok {
				base = at(base.Year(), base.Month(), base.Day(), h, mi, 0)
				consumed = 2
			} else if h, ok := parseHour(fields[1]); ok {
				base = at(base.Year(), base.Month(), base.Day(), h, 0, 0)
				consumed = 2
			}
		}
		return base, consumed, "завтра в " + base.Format("15:04"), nil
	}

	// @через N мин
	if first == "через" && len(fields) >= 3 && fields[2] == "мин" {
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 0 {
			return time.Time{}, 0, "", fmt.Errorf("не понял количество минут")
		}
		t := now.Add(time.Duration(n) * time.Minute)
		return t, 3, "через " + fields[1] + " мин", nil
	}

	// @ДД.ММ [HH:MM | HH]
	if d, mo, ok := parseDate(first); ok {
		h, mi := 9, 0
		consumed := 1
		if len(fields) > 1 {
			if hh, mm, ok2 := parseHHMM(fields[1]); ok2 {
				h, mi = hh, mm
				consumed = 2
			} else if hh, ok2 := parseHour(fields[1]); ok2 {
				h = hh
				consumed = 2
			}
		}
		t := at(now.Year(), time.Month(mo), d, h, mi, 0)
		if !t.After(now) {
			t = t.AddDate(1, 0, 0)
		}
		return t, consumed, fmt.Sprintf("%02d.%02d в %02d:%02d", d, mo, h, mi), nil
	}

	// @HH:MM [ДД.ММ | завтра]
	if h, mi, ok := parseHHMM(first); ok {
		t := at(now.Year(), now.Month(), now.Day(), h, mi, 0)
		consumed := 1
		day := "сегодня"
		if len(fields) > 1 {
			if d, mo, ok2 := parseDate(fields[1]); ok2 {
				t = at(now.Year(), time.Month(mo), d, h, mi, 0)
				if !t.After(now) {
					t = t.AddDate(1, 0, 0)
				}
				consumed = 2
				day = fmt.Sprintf("%02d.%02d", d, mo)
			} else if fields[1] == "завтра" {
				t = t.AddDate(0, 0, 1)
				consumed = 2
				day = "завтра"
			}
		}
		if day == "сегодня" && !t.After(now) {
			t = t.AddDate(0, 0, 1)
			day = "завтра"
		}
		return t, consumed, day + " в " + t.Format("15:04"), nil
	}

	// @HH
	if h, ok := parseHour(first); ok {
		t := at(now.Year(), now.Month(), now.Day(), h, 0, 0)
		day := "сегодня"
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
			day = "завтра"
		}
		return t, 1, fmt.Sprintf("%s в %02d:00", day, h), nil
	}

	return time.Time{}, 0, "", fmt.Errorf("не понял время — формат: @14:00, @14:30 завтра, @28.10 09:00")
}

func parseHHMM(s string) (int, int, bool) {
	m := reHHMM.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	if h > 23 || mi > 59 {
		return 0, 0, false
	}
	return h, mi, true
}

func parseHour(s string) (int, bool) {
	m := reHour.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	if h > 23 {
		return 0, false
	}
	return h, true
}

func parseDate(s string) (int, int, bool) {
	m := reDate.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	d, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	if d < 1 || d > 31 || mo < 1 || mo > 12 {
		return 0, 0, false
	}
	return d, mo, true
}
