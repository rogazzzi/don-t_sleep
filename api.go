package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hub рассылает события всем подключённым SSE-клиентам.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: map[chan []byte]struct{}{}}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default: // клиент не успевает читать — пропускаем кадр
		}
	}
}

// API — HTTP-слой приложения.
type API struct {
	store *Store
	cfg   *Config
	hub   *Hub
	web   fs.FS
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/api/notes", a.handleNotes)
	mux.HandleFunc("/api/notes/", a.handleNoteByID)
	mux.HandleFunc("/api/reminders", a.handleReminders)
	mux.HandleFunc("/api/reminders/", a.handleReminderByID)
	mux.HandleFunc("/api/capture", a.handleCapture)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/stream", a.handleStream)
	mux.HandleFunc("/", a.handleStatic)

	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, target)
}

func idFromPath(path, prefix string) (int64, error) {
	raw := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("некорректный идентификатор")
	}
	return id, nil
}

func (a *API) broadcast(kind string) {
	if a.hub != nil {
		a.hub.Broadcast(map[string]any{"type": kind, "action": "changed"})
	}
}

// ---------- обработчики ----------

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": appVersion})
}

func (a *API) handleNotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		notes, err := a.store.Notes(r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, notes)

	case http.MethodPost:
		var body struct {
			Text string `json:"text"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		note, err := a.store.CreateNote(body.Text)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.broadcast("note")
		writeJSON(w, http.StatusCreated, note)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) handleNoteByID(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/api/notes/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Pinned *bool `json:"pinned"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.Pinned != nil {
			if err := a.store.SetNotePinned(id, *body.Pinned); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
		}
		a.broadcast("note")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := a.store.DeleteNote(id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		a.broadcast("note")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) handleReminders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.store.Reminders()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, items)

	case http.MethodPost:
		var body struct {
			Text  string `json:"text"`
			DueAt int64  `json:"due_at"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := a.store.CreateReminder(body.Text, body.DueAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.broadcast("reminder")
		writeJSON(w, http.StatusCreated, item)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) handleReminderByID(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/api/reminders/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Done *bool `json:"done"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.Done != nil {
			if err := a.store.SetReminderDone(id, *body.Done); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
		}
		a.broadcast("reminder")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := a.store.DeleteReminder(id); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		a.broadcast("reminder")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type captureResponse struct {
	Kind     string    `json:"kind"`
	Text     string    `json:"text"`
	DueAt    int64     `json:"due_at,omitempty"`
	DueLabel string    `json:"due_label,omitempty"`
	Error    string    `json:"error,omitempty"`
	Note     *Note     `json:"note,omitempty"`
	Reminder *Reminder `json:"reminder,omitempty"`
}

func (a *API) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	res := ParseCapture(body.Text, time.Now())
	switch res.Kind {
	case "error":
		writeJSON(w, http.StatusBadRequest, captureResponse{Kind: "error", Error: res.Error, Text: body.Text})

	case "note":
		note, err := a.store.CreateNote(res.Text)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.broadcast("note")
		writeJSON(w, http.StatusCreated, captureResponse{Kind: "note", Note: &note})

	default:
		item, err := a.store.CreateReminder(res.Text, res.DueAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.broadcast("reminder")
		writeJSON(w, http.StatusCreated, captureResponse{
			Kind: "reminder", Reminder: &item, DueAt: res.DueAt, DueLabel: res.DueLabel,
		})
	}
}

func (a *API) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s := a.cfg.Get()
		writeJSON(w, http.StatusOK, map[string]any{
			"autostart": s.Autostart,
			"hotkey":    s.Hotkey,
			"version":   appVersion,
		})

	case http.MethodPatch:
		var body struct {
			Autostart *bool `json:"autostart"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.Autostart != nil {
			if err := a.cfg.Update(func(s *Settings) { s.Autostart = *body.Autostart }); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if err := syncAutostart(*body.Autostart); err != nil {
				log.Printf("автозапуск: %v", err)
			}
		}
		s := a.cfg.Get()
		writeJSON(w, http.StatusOK, map[string]any{
			"autostart": s.Autostart,
			"hotkey":    s.Hotkey,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.hub.subscribe()
	defer a.hub.unsubscribe(ch)

	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (a *API) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	http.FileServer(http.FS(a.web)).ServeHTTP(w, r)
}
