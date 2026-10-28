package main

import (
	"fmt"
	"log"
	"time"
)

// startScheduler раз в 20 секунд проверяет, не наступило ли время напоминаний.
func startScheduler(store *Store, hub *Hub) {
	go func() {
		check := func() { fireDue(store, hub) }
		check()

		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			check()
		}
	}()
}

func fireDue(store *Store, hub *Hub) {
	due, err := store.DueReminders(time.Now().Unix())
	if err != nil {
		log.Printf("проверка напоминаний: %v", err)
		return
	}
	if len(due) == 0 {
		return
	}

	ids := make([]int64, 0, len(due))
	for _, r := range due {
		ids = append(ids, r.ID)
	}
	if err := store.MarkRemindersFired(ids); err != nil {
		log.Printf("отметка напоминаний: %v", err)
	}

	user32MessageBeep.Call(MB_ICONEXCLAMATION)

	if len(due) == 1 {
		showToast("⏰ "+due[0].Text, alarmBrush)
	} else {
		showToast(fmt.Sprintf("⏰ %d напоминаний", len(due)), alarmBrush)
	}

	if hub != nil {
		hub.Broadcast(map[string]any{"type": "reminders", "action": "fired"})
	}
}
