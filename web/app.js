/* don't sleep — интерфейс */

const $ = (sel, root = document) => root.querySelector(sel);

const state = {
  tab: "notes",
  notes: [],
  reminders: [],
  q: "",
  settings: { autostart: false, hotkey: "Ctrl + Shift + Space" },
};

const MONTHS = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];

const ICONS = {
  check:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"><path d="m4.5 12.5 4.7 4.7L19.5 6.8"/></svg>',
  pin:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9.5 3.6h5l-.8 5.2 3.1 3.1v1.5H7.2v-1.5l3.1-3.1z"/><path d="M12 13.4V21"/></svg>',
  trash:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 6.5h15M9.5 6.5V4.4h5v2.1M7 6.5l.9 13.1h8.2L17 6.5"/><path d="M10.4 10.2v5.8M13.6 10.2v5.8"/></svg>',
  note:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3.5h9.5L19 8v12.5H5z"/><path d="M14 3.7V8h4.3"/><path d="M8.4 12.4h7.2M8.4 16h5"/></svg>',
  clock:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8.4"/><path d="M12 7.4V12l3.1 1.9"/></svg>',
  moon:
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.6A8.6 8.6 0 1 1 9.4 4a6.9 6.9 0 0 0 10.6 10.6z"/></svg>',
};

function iconEl(name) {
  const tpl = document.createElement("template");
  tpl.innerHTML = ICONS[name];
  return tpl.content.firstElementChild;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

/* ---------- сеть ---------- */

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (!res.ok) {
    let message = "Ошибка " + res.status;
    try {
      const body = await res.json();
      if (body && body.error) message = body.error;
    } catch (_) {}
    throw new Error(message);
  }
  if (res.status === 204) return null;
  const text = await res.text();
  return text ? JSON.parse(text) : null;
}

function tagsOf(note) {
  if (Array.isArray(note.tags)) return note.tags.filter(Boolean);
  if (typeof note.tags === "string") {
    return note.tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
  }
  return [];
}

const asBool = (v) => v === true || v === 1 || v === "1";

/* ---------- даты ---------- */

const pad = (n) => String(n).padStart(2, "0");
const fmtTime = (d) => pad(d.getHours()) + ":" + pad(d.getMinutes());

function startOfDay(d) {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

function dayDiff(from, to) {
  return Math.round((startOfDay(to) - startOfDay(from)) / 86400000);
}

function fmtDue(ts, done) {
  const d = new Date(ts * 1000);
  const now = new Date();
  const diff = dayDiff(now, d);
  const time = fmtTime(d);
  const overdue = !done && d.getTime() < now.getTime();

  let text;
  if (diff === 0) text = "сегодня, " + time;
  else if (diff === 1) text = "завтра, " + time;
  else if (diff === -1) text = "вчера, " + time;
  else if (diff > 1 && diff < 14) text = "через " + diff + " дн., " + time;
  else if (diff < -1 && diff > -14) text = Math.abs(diff) + " дн. назад, " + time;
  else text = d.getDate() + " " + MONTHS[d.getMonth()] + ", " + time;

  if (overdue && diff <= 0) text = "просрочено · " + time;
  return { text, overdue };
}

/* ---------- загрузка данных ---------- */

async function loadNotes() {
  state.notes = (await api("/api/notes")) || [];
}

async function loadReminders() {
  state.reminders = (await api("/api/reminders")) || [];
}

async function loadSettings() {
  try {
    const s = await api("/api/settings");
    if (s) state.settings = { ...state.settings, ...s };
  } catch (_) {}
}

let refreshTimer = null;
function refresh() {
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(async () => {
    try {
      await Promise.all([loadNotes(), loadReminders()]);
      render();
    } catch (_) {}
  }, 120);
}

/* ---------- отрисовка ---------- */

function render() {
  const openReminders = state.reminders.filter((r) => !asBool(r.done)).length;
  $("#badge-notes").textContent = state.notes.length;
  $("#badge-reminders").textContent = openReminders;

  if (state.tab === "notes") renderNotes();
  else renderReminders();

  syncSettingsUI();
}

function groupEl(title, count, cls) {
  const group = el("section", "group" + (cls ? " " + cls : ""));
  const head = el("div", "group-head");
  head.append(el("span", "dot"), el("span", "", title));
  if (count !== undefined) head.append(el("span", "group-count", String(count)));
  group.append(head);
  const list = el("div", "list");
  group.append(list);
  return { group, list };
}

function emptyState(icon, title, text) {
  const box = el("div", "empty");
  const mark = el("div", "empty-icon");
  mark.append(iconEl(icon));
  box.append(mark, el("div", "empty-title", title), el("div", "empty-text", text));
  return box;
}

function noteCard(note) {
  const card = el("div", "card" + (asBool(note.pinned) ? " pinned" : ""));
  card.dataset.noteId = note.id;

  const mark = el("div", "note-mark");
  mark.append(iconEl("note"));
  card.append(mark);

  const body = el("div", "card-body");
  body.append(el("div", "card-text", note.text || ""));

  const tags = tagsOf(note);
  if (tags.length) {
    const meta = el("div", "card-meta");
    tags.forEach((tag) => meta.append(el("span", "tag", "#" + tag)));
    body.append(meta);
  }
  card.append(body);

  const actions = el("div", "card-actions");

  const pinBtn = el("button", "mini-btn pinned" + (asBool(note.pinned) ? " pinned" : ""));
  pinBtn.title = asBool(note.pinned) ? "Открепить" : "Закрепить";
  pinBtn.append(iconEl("pin"));
  if (asBool(note.pinned)) pinBtn.style.color = "var(--accent)";
  pinBtn.addEventListener("click", async () => {
    try {
      await api("/api/notes/" + note.id, {
        method: "PATCH",
        body: JSON.stringify({ pinned: !asBool(note.pinned) }),
      });
      await loadNotes();
      render();
    } catch (err) {
      flashStatus(err.message, true);
    }
  });

  const delBtn = el("button", "mini-btn danger");
  delBtn.title = "Удалить";
  delBtn.append(iconEl("trash"));
  delBtn.addEventListener("click", async () => {
    try {
      await api("/api/notes/" + note.id, { method: "DELETE" });
      state.notes = state.notes.filter((n) => n.id !== note.id);
      render();
    } catch (err) {
      flashStatus(err.message, true);
    }
  });

  actions.append(pinBtn, delBtn);
  card.append(actions);
  return card;
}

function reminderCard(reminder) {
  const done = asBool(reminder.done);
  const card = el("div", "card" + (done ? " done" : ""));
  card.dataset.reminderId = reminder.id;

  const check = el("button", "check");
  check.title = done ? "Вернуть в работу" : "Отметить выполненным";
  check.append(iconEl("check"));
  check.addEventListener("click", async () => {
    try {
      await api("/api/reminders/" + reminder.id, {
        method: "PATCH",
        body: JSON.stringify({ done: !done }),
      });
      await loadReminders();
      render();
    } catch (err) {
      flashStatus(err.message, true);
    }
  });
  card.append(check);

  const body = el("div", "card-body");
  body.append(el("div", "card-text", reminder.text || ""));

  const due = fmtDue(Number(reminder.due_at) || 0, done);
  const meta = el("div", "card-meta");
  const chip = el("span", "due" + (due.overdue ? " overdue" : ""));
  chip.append(iconEl("clock"), document.createTextNode(due.text));
  meta.append(chip);
  body.append(meta);
  card.append(body);

  const actions = el("div", "card-actions");
  const delBtn = el("button", "mini-btn danger");
  delBtn.title = "Удалить";
  delBtn.append(iconEl("trash"));
  delBtn.addEventListener("click", async () => {
    try {
      await api("/api/reminders/" + reminder.id, { method: "DELETE" });
      state.reminders = state.reminders.filter((r) => r.id !== reminder.id);
      render();
    } catch (err) {
      flashStatus(err.message, true);
    }
  });
  actions.append(delBtn);
  card.append(actions);

  return card;
}

function renderNotes() {
  const host = $("#notes-list");
  host.textContent = "";

  const q = state.q.trim().toLowerCase();
  const visible = q
    ? state.notes.filter((n) => {
        const hay = ((n.text || "") + " " + tagsOf(n).join(" ")).toLowerCase();
        return hay.includes(q);
      })
    : state.notes;

  if (!visible.length) {
    host.append(
      q
        ? emptyState("note", "Ничего не нашлось", "Попробуй другой запрос или очисти поиск.")
        : emptyState("note", "Пока пусто", "Напиши что-нибудь в поле выше — или нажми Ctrl+Shift+Space из любого приложения.")
    );
    return;
  }

  const pinned = visible.filter((n) => asBool(n.pinned));
  const other = visible.filter((n) => !asBool(n.pinned));

  if (pinned.length) {
    const { group, list } = groupEl("Закреплённые", pinned.length);
    pinned.forEach((n) => list.append(noteCard(n)));
    host.append(group);
  }

  if (other.length) {
    const { group, list } = groupEl(pinned.length ? "Все заметки" : "Заметки", other.length);
    other.forEach((n) => list.append(noteCard(n)));
    host.append(group);
  }
}

function renderReminders() {
  const host = $("#reminders-list");
  host.textContent = "";

  const open = state.reminders
    .filter((r) => !asBool(r.done))
    .sort((a, b) => Number(a.due_at) - Number(b.due_at));
  const done = state.reminders
    .filter((r) => asBool(r.done))
    .sort((a, b) => Number(b.due_at) - Number(a.due_at))
    .slice(0, 40);

  if (!open.length && !done.length) {
    host.append(
      emptyState(
        "clock",
        "Напоминаний нет",
        "Начни запись с времени — например, «@18:00 позвонить в банк». Время можно задать и через «@завтра 9:30»."
      )
    );
    return;
  }

  const now = Date.now();
  const buckets = new Map([
    ["overdue", { title: "Просрочено", items: [] }],
    ["today", { title: "Сегодня", items: [] }],
    ["tomorrow", { title: "Завтра", items: [] }],
    ["later", { title: "Позже", items: [] }],
  ]);

  open.forEach((r) => {
    const ts = Number(r.due_at) * 1000;
    if (ts < now) buckets.get("overdue").items.push(r);
    else {
      const diff = dayDiff(new Date(), new Date(ts));
      buckets.get(diff <= 0 ? "today" : diff === 1 ? "tomorrow" : "later").items.push(r);
    }
  });

  for (const key of ["overdue", "today", "tomorrow", "later"]) {
    const bucket = buckets.get(key);
    if (!bucket.items.length) continue;
    const { group, list } = groupEl(
      bucket.title,
      bucket.items.length,
      key === "overdue" ? "overdue" : ""
    );
    bucket.items.forEach((r) => list.append(reminderCard(r)));
    host.append(group);
  }

  if (done.length) {
    const { group, list } = groupEl("Выполнено", done.length, "done");
    done.forEach((r) => list.append(reminderCard(r)));
    host.append(group);
  }
}

/* ---------- статус в подвале ---------- */

let statusTimer = null;
function flashStatus(message, isError = false) {
  const node = $("#footer-status");
  node.textContent = message;
  node.style.color = isError ? "var(--danger)" : "var(--ok)";
  clearTimeout(statusTimer);
  statusTimer = setTimeout(() => {
    node.textContent = "всё локально";
    node.style.color = "";
  }, 4200);
}

/* ---------- настройки ---------- */

function syncSettingsUI() {
  const on = asBool(state.settings.autostart);
  $("#switch-autostart").setAttribute("aria-checked", String(on));
  $("#hotkey-label").textContent = state.settings.hotkey || "Ctrl + Shift + Space";
}

function openSettings() {
  $("#settings-modal").classList.remove("hidden");
  loadSettings().then(syncSettingsUI);
}

function closeSettings() {
  $("#settings-modal").classList.add("hidden");
}

/* ---------- события ---------- */

function switchTab(tab) {
  state.tab = tab;
  document.querySelectorAll(".tab").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.tab === tab);
  });
  $("#view-notes").classList.toggle("hidden", tab !== "notes");
  $("#view-reminders").classList.toggle("hidden", tab !== "reminders");
  render();
}

function focusItem(kind, id) {
  if (kind === "reminder") switchTab("reminders");
  else switchTab("notes");

  const selector = kind === "reminder" ? `[data-reminder-id="${id}"]` : `[data-note-id="${id}"]`;
  const card = document.querySelector(selector);
  if (!card) return;
  card.scrollIntoView({ block: "center", behavior: "smooth" });
  card.classList.remove("flash");
  void card.offsetWidth;
  card.classList.add("flash");
}

function bindEvents() {
  $("#tabs").addEventListener("click", (event) => {
    const btn = event.target.closest(".tab");
    if (btn) switchTab(btn.dataset.tab);
  });

  $("#composer").addEventListener("submit", async (event) => {
    event.preventDefault();
    const input = $("#composer-input");
    const text = input.value.trim();
    if (!text) return;

    try {
      const result = await api("/api/capture", {
        method: "POST",
        body: JSON.stringify({ text }),
      });
      input.value = "";
      input.focus();
      const label = result && result.due_label ? result.due_label : "";
      if (result && result.kind === "reminder") {
        flashStatus("Напоминание: " + (label || "создано"));
      } else {
        flashStatus("Заметка сохранена");
      }
      refresh();
    } catch (err) {
      flashStatus(err.message, true);
    }
  });

  let searchTimer = null;
  $("#search").addEventListener("input", (event) => {
    clearTimeout(searchTimer);
    const value = event.target.value;
    searchTimer = setTimeout(() => {
      state.q = value;
      renderNotes();
    }, 90);
  });

  $("#btn-settings").addEventListener("click", openSettings);
  $("#btn-settings-close").addEventListener("click", closeSettings);
  $("#settings-modal").addEventListener("click", (event) => {
    if (event.target === $("#settings-modal")) closeSettings();
  });

  $("#switch-autostart").addEventListener("click", async () => {
    const next = !asBool(state.settings.autostart);
    try {
      await api("/api/settings", {
        method: "PATCH",
        body: JSON.stringify({ autostart: next }),
      });
      state.settings.autostart = next;
      syncSettingsUI();
      flashStatus(next ? "Автозапуск включён" : "Автозапуск выключен");
    } catch (err) {
      flashStatus(err.message, true);
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeSettings();
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
      event.preventDefault();
      switchTab("notes");
      $("#search").focus();
      $("#search").select();
    }
  });
}

/* ---------- поток событий ---------- */

function bindStream() {
  if (!window.EventSource) return;
  const stream = new EventSource("/api/stream");

  stream.addEventListener("note", refresh);
  stream.addEventListener("reminder", refresh);
  stream.addEventListener("focus", (event) => {
    try {
      const data = JSON.parse(event.data || "{}");
      refresh();
      if (data && data.id) setTimeout(() => focusItem(data.kind, data.id), 260);
    } catch (_) {}
  });
  stream.onmessage = refresh;
  stream.onerror = () => {};
}

/* ---------- старт ---------- */

async function start() {
  bindEvents();
  bindStream();
  try {
    await Promise.all([loadNotes(), loadReminders(), loadSettings()]);
  } catch (err) {
    flashStatus("Нет связи с приложением", true);
  }
  render();
  $("#composer-input").focus();
}

start();
