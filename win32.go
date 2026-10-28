package main

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Прямые биндинги к нужным функциям Win32 (без CGO).
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	user32GetMessageW            = user32.NewProc("GetMessageW")
	user32TranslateMessage       = user32.NewProc("TranslateMessage")
	user32DispatchMessageW       = user32.NewProc("DispatchMessageW")
	user32PeekMessageW           = user32.NewProc("PeekMessageW")
	user32PostQuitMessage        = user32.NewProc("PostQuitMessage")
	user32PostMessageW           = user32.NewProc("PostMessageW")
	user32RegisterClassExW       = user32.NewProc("RegisterClassExW")
	user32CreateWindowExW        = user32.NewProc("CreateWindowExW")
	user32DefWindowProcW         = user32.NewProc("DefWindowProcW")
	user32DestroyWindow          = user32.NewProc("DestroyWindow")
	user32RegisterHotKey         = user32.NewProc("RegisterHotKey")
	user32UnregisterHotKey       = user32.NewProc("UnregisterHotKey")
	user32ShowWindow             = user32.NewProc("ShowWindow")
	user32IsWindowVisible        = user32.NewProc("IsWindowVisible")
	user32SetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	user32GetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	user32SetFocus               = user32.NewProc("SetFocus")
	user32SetActiveWindow        = user32.NewProc("SetActiveWindow")
	user32GetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	user32AttachThreadInput      = user32.NewProc("AttachThreadInput")
	user32SetWindowTextW         = user32.NewProc("SetWindowTextW")
	user32GetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	user32GetWindowTextW         = user32.NewProc("GetWindowTextW")
	user32SetWindowLongPtrW      = user32.NewProc("SetWindowLongPtrW")
	user32GetWindowLongPtrW      = user32.NewProc("GetWindowLongPtrW")
	user32CallWindowProcW        = user32.NewProc("CallWindowProcW")
	user32SetWindowPos           = user32.NewProc("SetWindowPos")
	user32GetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	user32GetCursorPos           = user32.NewProc("GetCursorPos")
	user32MonitorFromPoint       = user32.NewProc("MonitorFromPoint")
	user32GetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	user32SetTimer               = user32.NewProc("SetTimer")
	user32KillTimer              = user32.NewProc("KillTimer")
	user32GetWindowRect          = user32.NewProc("GetWindowRect")
	user32GetClientRect          = user32.NewProc("GetClientRect")
	user32MessageBeep            = user32.NewProc("MessageBeep")
	user32CreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	user32AppendMenuW            = user32.NewProc("AppendMenuW")
	user32TrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	user32DestroyMenu            = user32.NewProc("DestroyMenu")
	user32CreateSolidBrush       = user32.NewProc("CreateSolidBrush")
	user32DeleteObject           = user32.NewProc("DeleteObject")
	user32GetStockObject         = user32.NewProc("GetStockObject")
	user32FillRect               = user32.NewProc("FillRect")
	user32FrameRect              = user32.NewProc("FrameRect")
	user32DrawTextW              = user32.NewProc("DrawTextW")
	user32SetBkMode              = user32.NewProc("SetBkMode")
	user32SetTextColor           = user32.NewProc("SetTextColor")
	user32SetBkColor             = user32.NewProc("SetBkColor")
	user32SelectObject           = user32.NewProc("SelectObject")
	user32SendMessageW           = user32.NewProc("SendMessageW")
	user32BeginPaint             = user32.NewProc("BeginPaint")
	user32EndPaint               = user32.NewProc("EndPaint")
	user32InvalidateRect         = user32.NewProc("InvalidateRect")
	user32CreateFontIndirectW    = user32.NewProc("CreateFontIndirectW")
	user32LoadCursorW            = user32.NewProc("LoadCursorW")
	user32LoadImageW             = user32.NewProc("LoadImageW")

	kernel32GetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	shell32ShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	dwmapiDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// ---------- константы ----------

const (
	WM_DESTROY        = 0x0002
	WM_CLOSE          = 0x0010
	WM_ACTIVATE       = 0x0006
	WM_SETFOCUS       = 0x0007
	WM_KILLFOCUS      = 0x0008
	WM_PAINT          = 0x000F
	WM_ERASEBKGND     = 0x0014
	WM_CTLCOLOREDIT   = 0x0133
	WM_COMMAND        = 0x0111
	WM_TIMER          = 0x0113
	WM_KEYDOWN        = 0x0100
	WM_CHAR           = 0x0102
	WM_LBUTTONUP      = 0x0202
	WM_CONTEXTMENU    = 0x007B
	WM_RBUTTONUP      = 0x0205
	WM_HOTKEY         = 0x0312
	WM_SETFONT        = 0x0030
	WM_USER           = 0x0400
	WM_QUIT           = 0x0012

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_POPUP            = 0x80000000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_TABSTOP          = 0x00010000
	WS_CLIPCHILDREN     = 0x02000000
	ES_AUTOHSCROLL      = 0x0080
	ES_NOHIDESEL        = 0x0100

	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_NOACTIVATE  = 0x08000000
	WS_EX_CLIENTEDGE  = 0x00000200

	SW_HIDE           = 0
	SW_SHOW           = 5
	SW_SHOWNOACTIVATE = 4
	SW_RESTORE        = 9

	GWLP_WNDPROC = ^uintptr(3) // -4

	MOD_CONTROL = 0x0002
	MOD_SHIFT   = 0x0004
	VK_SPACE    = 0x20
	VK_RETURN   = 0x0D
	VK_ESCAPE   = 0x1B

	WA_INACTIVE = 0

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	TPM_LEFTALIGN = 0x0000
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD = 0x0100

	EM_SETCUEBANNER = 0x1501

	NIM_ADD    = 0
	NIM_DELETE = 2
	NIF_MESSAGE = 1
	NIF_ICON    = 2
	NIF_TIP     = 4

	IDI_APPLICATION = 32512
	IMAGE_ICON      = 1
	LR_SHARED       = 0x8000
	LR_DEFAULTSIZE  = 0x0040

	IDC_ARROW = 32512

	MONITOR_DEFAULTTONEAREST = 2

	HWND_TOPMOST = ^uintptr(0) // -1
	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010
	SWP_SHOWWINDOW = 0x0040

	DT_LEFT         = 0x0000
	DT_SINGLELINE   = 0x0020
	DT_VCENTER      = 0x0004
	DT_END_ELLIPSIS = 0x8000
	DT_NOPREFIX     = 0x0800

	TRANSPARENT = 1

	MB_ICONEXCLAMATION = 0x00000030
)

// ---------- структуры ----------

type WndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct{ X, Y int32 }

type RECT struct {
	Left, Top, Right, Bottom int32
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type LOGFONTW struct {
	LfHeight         int32
	LfWidth          int32
	LfEscapement     int32
	LfOrientation    int32
	LfWeight         int32
	LfItalic         byte
	LfUnderline      byte
	LfStrikeOut      byte
	LfCharSet        byte
	LfOutPrecision   byte
	LfClipPrecision  byte
	LfQuality        byte
	LfPitchAndFamily byte
	LfFaceName       [32]uint16
}

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// ---------- мелкие помощники ----------

func rgb(r, g, b byte) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

var (
	clsHidden  = windows.StringToUTF16("dontsleep_hidden")
	clsCapture = windows.StringToUTF16("dontsleep_capture")
	clsToast   = windows.StringToUTF16("dontsleep_toast")
	clsEdit    = windows.StringToUTF16("EDIT")
	emptyStr   = []uint16{0}
)

func u16ptr(v []uint16) uintptr {
	if len(v) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&v[0]))
}

// utf16Call временно держит UTF-16 строку живой на время вызова Win32.
func utf16Call(s string, fn func(p uintptr)) {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	fn(uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}
