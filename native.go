package main

import (
	"log"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hotkeyID = 1

	WM_TRAYICON       = WM_USER + 1
	WM_CAPTURE_SUBMIT = WM_USER + 10
	WM_CAPTURE_CANCEL = WM_USER + 11
	WM_NOTIFY_TOAST   = WM_USER + 20

	toastTimerID = 7

	IDM_OPEN    = 1001
	IDM_CAPTURE = 1002
	IDM_EXIT    = 1003

	EM_SETSEL = 0x00B1
)

var (
	hiddenProcCb  uintptr
	captureProcCb uintptr
	toastProcCb   uintptr
	editProcCb    uintptr
	mainProcCb    uintptr

	hiddenHwnd  uintptr
	captureHwnd uintptr
	captureEdit uintptr
	toastHwnd   uintptr
	mainHwnd    uintptr

	origEditProc uintptr
	origMainProc uintptr

	hInstance uintptr
	scaleF    = 1.0

	uiFont      uintptr
	darkBrush   uintptr
	editBrush   uintptr
	accentBrush uintptr
	okBrush     uintptr
	warnBrush   uintptr
	alarmBrush  uintptr

	textColor uintptr
	bgColor   uintptr

	appURL   string
	appStore *Store
	appHub   *Hub
	appCfg   *Config

	captureOpen bool
	exiting     bool

	toastText   string
	toastAccent uintptr

	notifyMu    sync.Mutex
	notifyQueue []toastItem
)

type toastItem struct {
	text     string
	accent   uintptr
	duration int
}

// ---------- настройка ----------

func initDPI() {
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		p.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		return
	}
	if p := shcore.NewProc("SetProcessDpiAwareness"); p.Find() == nil {
		p.Call(2) // PROCESS_PER_MONITOR_DPI_AWARE
		return
	}
	user32.NewProc("SetProcessDPIAware").Call()
}

func systemDPI() int {
	if p := user32.NewProc("GetDpiForSystem"); p.Find() == nil {
		if r, _, _ := p.Call(); r != 0 {
			return int(r)
		}
	}
	return 96
}

func createFont(face string, height, weight int32) uintptr {
	var lf LOGFONTW
	lf.LfHeight = height
	lf.LfWeight = weight
	lf.LfCharSet = 1  // DEFAULT_CHARSET
	lf.LfQuality = 5  // CLEARTYPE_QUALITY
	copy(lf.LfFaceName[:], windows.StringToUTF16(face))
	f, _, _ := gdi32CreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return f
}

func createSolid(color uintptr) uintptr {
	b, _, _ := gdi32CreateSolidBrush.Call(color)
	return b
}

func registerClass(name []uint16, proc, bg, cursor uintptr) {
	wc := WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(WndClassExW{})),
		LpfnWndProc:   proc,
		HInstance:     hInstance,
		HCursor:       cursor,
		HbrBackground: bg,
		LpszClassName: (*uint16)(unsafe.Pointer(&name[0])),
	}
	user32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// setupNative создаёт весь нативный слой. Должен вызываться на главном потоке.
func setupNative(url string, store *Store, cfg *Config, hub *Hub) {
	appURL = url
	appStore = store
	appCfg = cfg
	appHub = hub

	hInstance, _, _ = kernel32GetModuleHandleW.Call(0)

	initDPI()
	scaleF = float64(systemDPI()) / 96.0
	if scaleF < 1 {
		scaleF = 1
	}

	textColor = rgb(232, 234, 240)
	bgColor = rgb(18, 20, 26)

	uiFont = createFont("Segoe UI", int32(-17*scaleF), 400)
	darkBrush = createSolid(bgColor)
	editBrush = createSolid(bgColor)
	accentBrush = createSolid(rgb(246, 196, 83))
	okBrush = createSolid(rgb(52, 211, 153))
	warnBrush = createSolid(rgb(251, 191, 36))
	alarmBrush = createSolid(rgb(248, 113, 113))

	hiddenProcCb = windows.NewCallback(winHiddenProc)
	captureProcCb = windows.NewCallback(winCaptureProc)
	toastProcCb = windows.NewCallback(winToastProc)
	editProcCb = windows.NewCallback(winEditProc)
	mainProcCb = windows.NewCallback(winMainProc)

	cursor, _, _ := user32LoadCursorW.Call(0, IDC_ARROW)
	registerClass(clsHidden, hiddenProcCb, 0, cursor)
	registerClass(clsCapture, captureProcCb, darkBrush, cursor)
	registerClass(clsToast, toastProcCb, darkBrush, cursor)

	createHiddenWindow()
	createCaptureWindow()
	createToastWindow()

	addTrayIcon()
	registerHotkey()
}

func createHiddenWindow() {
	hiddenHwnd, _, _ = user32CreateWindowExW.Call(0, u16ptr(clsHidden), u16ptr(clsHidden),
		0, 0, 0, 0, 0, 0, 0, hInstance, 0)
}

func createCaptureWindow() {
	w := uintptr(620 * scaleF)
	h := uintptr(54 * scaleF)
	captureHwnd, _, _ = user32CreateWindowExW.Call(
		uintptr(WS_EX_TOPMOST|WS_EX_TOOLWINDOW),
		u16ptr(clsCapture), u16ptr(clsCapture),
		uintptr(WS_POPUP|WS_CLIPCHILDREN),
		0, 0, w, h, 0, 0, hInstance, 0)

	m := uintptr(8 * scaleF)
	captureEdit, _, _ = user32CreateWindowExW.Call(
		0, u16ptr(clsEdit), 0,
		uintptr(WS_CHILD|WS_VISIBLE|ES_AUTOHSCROLL|ES_NOHIDESEL),
		m, m, w-2*m, h-2*m, captureHwnd, 0, hInstance, 0)

	user32SendMessageW.Call(captureEdit, WM_SETFONT, uiFont, 1)
	utf16Call("Заметка или @14:00 напоминание…", func(p uintptr) {
		user32SendMessageW.Call(captureEdit, EM_SETCUEBANNER, 1, p)
	})

	orig, _, _ := user32SetWindowLongPtrW.Call(captureEdit, GWLP_WNDPROC, editProcCb)
	origEditProc = orig
	if orig == 0 {
		log.Printf("не удалось перехватить обработку клавиш в поле ввода")
	}
}

func createToastWindow() {
	w := uintptr(360 * scaleF)
	h := uintptr(58 * scaleF)
	toastHwnd, _, _ = user32CreateWindowExW.Call(
		uintptr(WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE),
		u16ptr(clsToast), u16ptr(clsToast),
		uintptr(WS_POPUP),
		0, 0, w, h, 0, 0, hInstance, 0)
}

// ---------- цикл сообщений ----------

// runNativeLoop крутит общий цикл сообщений для всех нативных окон и WebView2.
func runNativeLoop() {
	var msg MSG
	for {
		ret, _, _ := user32GetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			return
		}
		if msg.Message == WM_QUIT {
			return
		}
		user32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		user32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// ---------- оконные процедуры ----------

func winHiddenProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_HOTKEY:
		if wp == hotkeyID {
			showCapture()
		}
		return 0

	case WM_TRAYICON:
		switch uint32(lp) & 0xFFFF {
		case WM_LBUTTONUP, 0x0400: // 0x0400 = NIN_SELECT
			toggleMainWindow()
		case WM_RBUTTONUP, WM_CONTEXTMENU:
			showTrayMenu()
		}
		return 0

	case WM_NOTIFY_TOAST:
		drainToasts()
		return 0

	case WM_CLOSE:
		requestExit()
		return 0
	}
	r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func winCaptureProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := user32BeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var rc RECT
		user32GetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		user32FillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), darkBrush)
		user32FrameRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), accentBrush)
		user32EndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0

	case WM_ERASEBKGND:
		return 1

	case WM_CTLCOLOREDIT:
		gdi32SetTextColor.Call(wp, textColor)
		gdi32SetBkColor.Call(wp, bgColor)
		return editBrush

	case WM_ACTIVATE:
		if wp == WA_INACTIVE {
			hideCapture()
		}
		return 0

	case WM_CAPTURE_SUBMIT:
		submitCapture()
		return 0

	case WM_CAPTURE_CANCEL:
		hideCapture()
		return 0
	}
	r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func winEditProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_KEYDOWN:
		switch wp {
		case VK_RETURN:
			user32PostMessageW.Call(captureHwnd, WM_CAPTURE_SUBMIT, 0, 0)
			return 0
		case VK_ESCAPE:
			user32PostMessageW.Call(captureHwnd, WM_CAPTURE_CANCEL, 0, 0)
			return 0
		}
	}
	if origEditProc == 0 {
		r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	return callWindowProc(origEditProc, hwnd, msg, wp, lp)
}

func winToastProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		paintToast(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_TIMER:
		if wp == toastTimerID {
			user32ShowWindow.Call(toastHwnd, SW_HIDE)
		}
		return 0
	case WM_LBUTTONUP:
		user32ShowWindow.Call(toastHwnd, SW_HIDE)
		return 0
	}
	r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func winMainProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == WM_CLOSE {
		if exiting {
			// настоящий выход: отдаём событие оригинальной процедуре
			if origMainProc == 0 {
				r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
				return r
			}
			return callWindowProc(origMainProc, hwnd, msg, wp, lp)
		}
		hideMainWindow()
		return 0
	}
	if origMainProc == 0 {
		r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	return callWindowProc(origMainProc, hwnd, msg, wp, lp)
}

func callWindowProc(proc, hwnd, msg, wp, lp uintptr) uintptr {
	r, _, _ := user32CallWindowProcW.Call(proc, hwnd, msg, wp, lp)
	return r
}

// ---------- горячая клавиша и трей ----------

func registerHotkey() {
	r, _, err := user32RegisterHotKey.Call(hiddenHwnd, hotkeyID, MOD_CONTROL|MOD_SHIFT, VK_SPACE)
	if r == 0 {
		log.Printf("не удалось занять Ctrl+Shift+Space: %v", err)
	}
}

func addTrayIcon() {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hiddenHwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAYICON
	icon, _, _ := user32LoadImageW.Call(0, IDI_APPLICATION, IMAGE_ICON, 0, 0, LR_SHARED|LR_DEFAULTSIZE)
	nid.HIcon = icon
	tip := windows.StringToUTF16("don't sleep — заметки и напоминания")
	copy(nid.SzTip[:], tip)
	shell32ShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
}

func removeTrayIcon() {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hiddenHwnd
	nid.UID = 1
	shell32ShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func showTrayMenu() {
	var pt POINT
	user32GetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	menu, _, _ := user32CreatePopupMenu.Call()
	utf16Call("Открыть", func(p uintptr) {
		user32AppendMenuW.Call(menu, MF_STRING, IDM_OPEN, p)
	})
	utf16Call("Быстрая заметка…", func(p uintptr) {
		user32AppendMenuW.Call(menu, MF_STRING, IDM_CAPTURE, p)
	})
	user32AppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	utf16Call("Выход", func(p uintptr) {
		user32AppendMenuW.Call(menu, MF_STRING, IDM_EXIT, p)
	})

	user32SetForegroundWindow.Call(hiddenHwnd)
	r, _, _ := user32TrackPopupMenu.Call(menu,
		TPM_LEFTALIGN|TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(int64(pt.X)), uintptr(int64(pt.Y)), 0, hiddenHwnd, 0)
	user32DestroyMenu.Call(menu)
	user32PostMessageW.Call(hiddenHwnd, 0, 0, 0) // WM_NULL

	switch r {
	case IDM_OPEN:
		showMainWindow()
	case IDM_CAPTURE:
		showCapture()
	case IDM_EXIT:
		requestExit()
	}
}

// ---------- окно быстрой записи ----------

func showCapture() {
	if captureHwnd == 0 {
		return
	}
	user32SetWindowTextW.Call(captureEdit, u16ptr(emptyStr))
	placeCapture()
	user32ShowWindow.Call(captureHwnd, SW_SHOW)
	forceForeground(captureHwnd)
	user32SetFocus.Call(captureEdit)
	captureOpen = true
}

func showCaptureText(text string) {
	showCapture()
	utf16Call(text, func(p uintptr) {
		user32SetWindowTextW.Call(captureEdit, p)
	})
	user32SendMessageW.Call(captureEdit, EM_SETSEL, 0, ^uintptr(0))
}

func hideCapture() {
	if captureHwnd == 0 {
		return
	}
	user32ShowWindow.Call(captureHwnd, SW_HIDE)
	captureOpen = false
}

func placeCapture() {
	w := int32(620 * scaleF)
	h := int32(54 * scaleF)
	work := cursorMonitorWork()
	x := work.Left + (work.Right-work.Left-w)/2
	y := work.Top + int32(90*scaleF)
	if y+h > work.Bottom {
		y = work.Top + int32(40*scaleF)
	}
	user32SetWindowPos.Call(captureHwnd, HWND_TOPMOST,
		uintptr(int64(x)), uintptr(int64(y)), uintptr(int64(w)), uintptr(int64(h)),
		SWP_NOACTIVATE)
}

func cursorMonitorWork() RECT {
	var pt POINT
	user32GetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	ptval := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	mon, _, _ := user32MonitorFromPoint.Call(ptval, MONITOR_DEFAULTTONEAREST)

	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := user32GetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		sx, _, _ := user32GetSystemMetrics.Call(0) // SM_CXSCREEN
		sy, _, _ := user32GetSystemMetrics.Call(1) // SM_CYSCREEN
		return RECT{0, 0, int32(sx), int32(sy)}
	}
	return mi.RcWork
}

func forceForeground(hwnd uintptr) {
	fg, _, _ := user32GetForegroundWindow.Call()
	if fg == hwnd {
		return
	}
	fgTid, _, _ := user32GetWindowThreadProcess.Call(fg, 0)
	myTid, _, _ := user32GetWindowThreadProcess.Call(hwnd, 0)
	attached := false
	if fgTid != 0 && myTid != 0 && fgTid != myTid {
		user32AttachThreadInput.Call(fgTid, myTid, 1)
		attached = true
	}
	user32SetForegroundWindow.Call(hwnd)
	user32SetActiveWindow.Call(hwnd)
	if attached {
		user32AttachThreadInput.Call(fgTid, myTid, 0)
	}
}

func getEditText(hwnd uintptr) string {
	n, _, _ := user32GetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	user32GetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func submitCapture() {
	text := strings.TrimSpace(getEditText(captureEdit))
	if text == "" {
		hideCapture()
		return
	}

	res := ParseCapture(text, time.Now())
	switch res.Kind {
	case "note":
		if _, err := appStore.CreateNote(res.Text); err != nil {
			log.Printf("заметка: %v", err)
			hideCapture()
			showToast("⚠ Не удалось сохранить заметку", warnBrush)
			return
		}
		hideCapture()
		showToast("✓ Заметка сохранена", okBrush)

	case "reminder":
		if _, err := appStore.CreateReminder(res.Text, res.DueAt); err != nil {
			log.Printf("напоминание: %v", err)
			hideCapture()
			showToast("⚠ Не удалось сохранить напоминание", warnBrush)
			return
		}
		hideCapture()
		showToast("⏰ "+res.DueLabel+" — "+res.Text, okBrush)

	case "error":
		// не закрываем окно: пусть пользователь поправит формат
		showToast("⚠ "+res.Error, warnBrush)
		return
	}

	if appHub != nil {
		appHub.Broadcast(map[string]any{"type": "capture", "kind": res.Kind})
	}
}

// ---------- главное окно ----------

func subclassMainWindow(hwnd uintptr) {
	mainHwnd = hwnd
	orig, _, _ := user32SetWindowLongPtrW.Call(hwnd, GWLP_WNDPROC, mainProcCb)
	origMainProc = orig
	if orig == 0 {
		log.Printf("не удалось перехватить закрытие главного окна")
		return
	}

	one := int32(1)
	dwmapiDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&one)), 4)
}

func showMainWindow() {
	if mainHwnd == 0 {
		openBrowser(appURL)
		return
	}
	user32ShowWindow.Call(mainHwnd, SW_RESTORE)
	forceForeground(mainHwnd)
}

func hideMainWindow() {
	if mainHwnd == 0 {
		return
	}
	user32ShowWindow.Call(mainHwnd, SW_HIDE)
}

func toggleMainWindow() {
	if mainHwnd == 0 {
		showMainWindow()
		return
	}
	if isVisible(mainHwnd) {
		hideMainWindow()
	} else {
		showMainWindow()
	}
}

func isVisible(hwnd uintptr) bool {
	r, _, _ := user32IsWindowVisible.Call(hwnd)
	return r != 0
}

// ---------- всплывающие уведомления ----------

// showToast можно звать из любой горутины: работа с окном уйдёт в UI-поток.
func showToast(text string, accent uintptr) {
	notifyMu.Lock()
	notifyQueue = append(notifyQueue, toastItem{text: text, accent: accent, duration: 2800})
	notifyMu.Unlock()
	if hiddenHwnd != 0 {
		user32PostMessageW.Call(hiddenHwnd, WM_NOTIFY_TOAST, 0, 0)
	}
}

func drainToasts() {
	notifyMu.Lock()
	items := notifyQueue
	notifyQueue = nil
	notifyMu.Unlock()
	for _, it := range items {
		showToastNow(it)
	}
}

func showToastNow(it toastItem) {
	toastText = it.text
	toastAccent = it.accent

	runes := len([]rune(it.text))
	w := int32(float64(runes)*7.4*scaleF) + int32(52*scaleF)
	if w < int32(260*scaleF) {
		w = int32(260 * scaleF)
	}
	if w > int32(560*scaleF) {
		w = int32(560 * scaleF)
	}
	h := int32(58 * scaleF)

	work := cursorMonitorWork()
	pad := int32(16 * scaleF)
	x := work.Right - w - pad
	y := work.Bottom - h - pad

	user32SetWindowPos.Call(toastHwnd, HWND_TOPMOST,
		uintptr(int64(x)), uintptr(int64(y)), uintptr(int64(w)), uintptr(int64(h)),
		SWP_NOACTIVATE|SWP_SHOWWINDOW)
	user32KillTimer.Call(toastHwnd, toastTimerID)
	user32SetTimer.Call(toastHwnd, toastTimerID, uintptr(it.duration), 0)
	user32InvalidateRect.Call(toastHwnd, 0, 1)
	user32ShowWindow.Call(toastHwnd, SW_SHOWNOACTIVATE)
}

func paintToast(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := user32BeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer user32EndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}

	var rc RECT
	user32GetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	user32FillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), darkBrush)
	bar := RECT{rc.Left, rc.Top, rc.Left + int32(4*scaleF), rc.Bottom}
	user32FillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), toastAccent)

	old, _, _ := gdi32SelectObject.Call(hdc, uiFont)
	gdi32SetBkMode.Call(hdc, TRANSPARENT)
	gdi32SetTextColor.Call(hdc, textColor)

	textRect := RECT{
		rc.Left + int32(18*scaleF),
		rc.Top,
		rc.Right - int32(12*scaleF),
		rc.Bottom,
	}
	flags := uintptr(DT_LEFT | DT_SINGLELINE | DT_VCENTER | DT_NOPREFIX | DT_END_ELLIPSIS)
	utf16Call(toastText, func(p uintptr) {
		user32DrawTextW.Call(hdc, p, ^uintptr(0), uintptr(unsafe.Pointer(&textRect)), flags)
	})
	gdi32SelectObject.Call(hdc, old)
}

// ---------- завершение ----------

func requestExit() {
	user32UnregisterHotKey.Call(hiddenHwnd, hotkeyID)
	removeTrayIcon()
	if mainHwnd != 0 {
		exiting = true
		// DestroyWindow → WM_DESTROY → библиотека делает PostQuitMessage
		user32DestroyWindow.Call(mainHwnd)
		return
	}
	user32PostQuitMessage.Call(0)
}
