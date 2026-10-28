(() => {
  const $ = (s) => document.querySelector(s);
  const state = { notes: [], reminders: [], tab: 'notes', search: '' };
  const DEFAULT_HINT = 'Enter — сохранить. Без @ — заметка, с @ — напоминание.';
  let hintTimer = 0;

  async function api(path, opts = {}) {
    const res = await fetch(path, {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    });
    if (res.status === 204) return null;
    const data = await res.json().catch(() => null);
    if (!res.ok) {
      throw new Error((data && (data.error || data.Error)) || 'Ошибка запроса');
    }
    return data;
  }

  function fmtDue(ts) {
    const d = new Date(ts * 1000);
    const now = new Date();
    const dayStart = (x) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
    const days = Math.round((dayStart(d) - dayStart(now)) / 86400000);
    const time = d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
    let day;
    if (days === 0) day = 'сегодня';
    else if (days === 1) day = 'завтра';
    else if (days === -1) day = 'вчера';
    else day = d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' });
    return `${day} в ${time}`;
  }

  function fmtStamp(ts) {
    return new Date(ts * 1000).toLocaleString('ru-RU', {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  function setHint(text, kind = '') {
    const el = $('#hint');
    el.textContent = text;
    el.className = 'hint ' + kind;
    clearTimeout(hintTimer);
    hintTimer = setTimeout(() => {
      el.textContent = DEFAULT_HINT;
      el.className = 'hint';
    }, 4200);
  }

  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text != null) node.textContent = text;
    return node;
  }

  // ---------- заметки ----------

  async function loadNotes() {
    const q = state.search ? '?q=' + encodeURIComponent(state.search) : '';
    state.notes = await api('/api/notes' + q);
    renderNotes();
  }

  function renderNotes() {
    const list = $('#notesList');
    list.innerHTML = '';
    const notes = state.notes;
    $('#notesEmpty').style.display = notes.length ? 'none' : 'block';
    $('#notesBadge').textContent = notes.length || '';

    for (const n of notes) {
      const card = el('article', 'card note' + (n.pinned ? ' pinned' : ''));

      const body = el('div', 'card-body');
      body.appendChild(el('div', 'card-text', n.text));

      const foot = el('div', 'card-foot');
      foot.appendChild(el('span', 'stamp', fmtStamp(n.created_at)));
      for (const tag of (n.tags || '').split(/\s+/).filter(Boolean)) {
        foot.appendChild(el('span', 'tag', tag));
      }
      body.appendChild(foot);
      card.appendChild(body);

      const actions = el('div', 'card-actions');
      const pin = el('button', 'icon-btn' + (n.pinned ? ' active' : ''), n.pinned ? '★' : '☆');
      pin.title = n.pinned ? 'Открепить' : 'Закрепить';
      pin.onclick = async () => {
        try {
          await api(`/api/notes/${n.id}`, { method: 'PATCH', body: JSON.stringify({ pinned: !n.pinned }) });
          await loadNotes();
        } catch (e) {
          setHint('⚠ ' + e.message, 'warn');
        }
      };
      const del = el('button', 'icon-btn danger', '✕');
      del.title = 'Удалить';
      del.onclick = async () => {
        try {
          await api(`/api/notes/${n.id}`, { method: 'DELETE' });
          await loadNotes();
        } catch (e) {
          setHint('⚠ ' + e.message, 'warn');
        }
      };
      actions.append(pin, del);
      card.appendChild(actions);

      list.appendChild(card);
    }
  }

  // ---------- напоминания ----------

  async function loadReminders() {
    state.reminders = await api('/api/reminders');
    renderReminders();
  }

  function renderReminders() {
    const list = $('#remindersList');
    list.innerHTML = '';
    const items = state.reminders;
    const open = items.filter((r) => !r.done);
    const done = items.filter((r) => r.done);
    $('#remindersEmpty').style.display = items.length ? 'none' : 'block';
    $('#remindersBadge').textContent = open.length || '';

    for (const r of [...open, ...done]) {
      const overdue = !r.done && r.due_at * 1000 < Date.now();
      const card = el('article', 'card reminder' + (r.done ? ' done' : '') + (overdue ? ' overdue' : ''));

      const check = el('label', 'check');
      const box = document.createElement('input');
      box.type = 'checkbox';
      box.checked = r.done;
      box.onchange = async () => {
        try {
          await api(`/api/reminders/${r.id}`, {
            method: 'PATCH',
            body: JSON.stringify({ done: box.checked }),
          });
          await loadReminders();
        } catch (e) {
          setHint('⚠ ' + e.message, 'warn');
        }
      };
      check.appendChild(box);
      card.appendChild(check);

      const body = el('div', 'card-body');
      body.appendChild(el('div', 'card-text', r.text));
      body.appendChild(el('div', 'due', (r.done ? 'сделано · ' : '') + fmtDue(r.due_at)));
      card.appendChild(body);

      const actions = el('div', 'card-actions');
      const del = el('button', 'icon-btn danger', '✕');
      del.title = 'Удалить';
      del.onclick = async () => {
        try {
          await api(`/api/reminders/${r.id}`, { method: 'DELETE' });
          await loadReminders();
        } catch (e) {
          setHint('⚠ ' + e.message, 'warn');
        }
      };
      actions.appendChild(del);
      card.appendChild(actions);

      list.appendChild(card);
    }
  }

  // ---------- быстрый ввод ----------

  async function submitQuick() {
    const input = $('#quickInput');
    const text = input.value.trim();
    if (!text) return;

    input.disabled = true;
    try {
      const res = await api('/api/capture', { method: 'POST', body: JSON.stringify({ text }) });
      input.value = '';
      if (res.kind === 'reminder') {
        setHint('⏰ Напоминание: ' + res.due_label, 'ok');
      } else {
        setHint('✓ Заметка сохранена', 'ok');
      }
      await Promise.all([loadNotes(), loadReminders()]);
    } catch (e) {
      setHint('⚠ ' + e.message, 'warn');
    } finally {
      input.disabled = false;
      input.focus();
    }
  }

  // ---------- вкладки ----------

  function switchTab(tab) {
    state.tab = tab;
    for (const btn of document.querySelectorAll('.tab')) {
      btn.classList.toggle('active', btn.dataset.tab === tab);
    }
    $('#notesView').classList.toggle('active', tab === 'notes');
    $('#remindersView').classList.toggle('active', tab === 'reminders');
  }

  // ---------- настройки ----------

  async function loadSettings() {
    const s = await api('/api/settings');
    $('#autostartToggle').checked = !!s.autostart;
    $('#hotkeyLabel').textContent = s.hotkey || 'Ctrl+Shift+Space';
    $('#versionLabel').textContent = 'v' + s.version;
  }

  // ---------- поток обновлений ----------

  function connectStream() {
    const es = new EventSource('/api/stream');
    es.onmessage = () => {
      loadNotes().catch(() => {});
      loadReminders().catch(() => {});
    };
  }

  // ---------- события ----------

  function bind() {
    for (const btn of document.querySelectorAll('.tab')) {
      btn.onclick = () => switchTab(btn.dataset.tab);
    }

    $('#addBtn').onclick = submitQuick;
    $('#quickInput').onkeydown = (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submitQuick();
      }
    };

    let searchTimer = 0;
    $('#searchInput').oninput = (e) => {
      clearTimeout(searchTimer);
      const q = e.target.value.trim();
      searchTimer = setTimeout(() => {
        state.search = q;
        loadNotes().catch(() => {});
      }, 180);
    };

    $('#settingsBtn').onclick = () => {
      $('#settingsModal').classList.remove('hidden');
      loadSettings().catch(() => {});
    };
    $('#closeSettings').onclick = () => $('#settingsModal').classList.add('hidden');
    $('#settingsModal').onclick = (e) => {
      if (e.target.id === 'settingsModal') $('#settingsModal').classList.add('hidden');
    };
    $('#autostartToggle').onchange = async (e) => {
      try {
        await api('/api/settings', {
          method: 'PATCH',
          body: JSON.stringify({ autostart: e.target.checked }),
        });
        setHint(e.target.checked ? '✓ Автозапуск включён' : 'Автозапуск выключен', 'ok');
      } catch (err) {
        setHint('⚠ ' + err.message, 'warn');
      }
    };

    $('#quickInput').focus();
  }

  async function init() {
    bind();
    try {
      await Promise.all([loadNotes(), loadReminders()]);
    } catch (e) {
      setHint('⚠ ' + e.message, 'warn');
    }
    connectStream();
  }

  init();
})();
