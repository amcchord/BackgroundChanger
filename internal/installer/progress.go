package installer

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	comctl32         = syscall.NewLazyDLL("comctl32.dll")
	gdi32            = syscall.NewLazyDLL("gdi32.dll")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procSendMessageW       = user32.NewProc("SendMessageW")
	procSetWindowTextW     = user32.NewProc("SetWindowTextW")
	procEnableWindow       = user32.NewProc("EnableWindow")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
)

// Window styles
const (
	WS_OVERLAPPED     = 0x00000000
	WS_CAPTION        = 0x00C00000
	WS_SYSMENU        = 0x00080000
	WS_VISIBLE        = 0x10000000
	WS_CHILD          = 0x40000000
	WS_BORDER         = 0x00800000
	WS_EX_CLIENTEDGE  = 0x00000200
	WS_DISABLED       = 0x08000000
	WS_MINIMIZEBOX    = 0x00020000

	CW_USEDEFAULT = 0x80000000

	SW_SHOW = 5

	WM_DESTROY = 0x0002
	WM_COMMAND = 0x0111
	WM_CLOSE   = 0x0010
	WM_USER    = 0x0400

	BN_CLICKED = 0

	BS_PUSHBUTTON = 0x00000000
	BS_DEFPUSHBUTTON = 0x00000001

	SS_LEFT = 0x00000000

	PBS_SMOOTH = 0x01

	PBM_SETRANGE = WM_USER + 1
	PBM_SETPOS   = WM_USER + 2
	PBM_SETSTEP  = WM_USER + 4
	PBM_STEPIT   = WM_USER + 5
	PBM_SETRANGE32 = WM_USER + 6

	ICC_PROGRESS_CLASS = 0x00000020

	LOGPIXELSY = 90

	PM_REMOVE = 0x0001

	// Custom message for updating progress from another goroutine
	WM_UPDATE_PROGRESS = WM_USER + 100
	WM_UPDATE_STATUS   = WM_USER + 101
	WM_ENABLE_CLOSE    = WM_USER + 102
	WM_SET_COMPLETE    = WM_USER + 103
)

// Control IDs
const (
	IDC_STATUS    = 1001
	IDC_PROGRESS  = 1002
	IDC_CLOSEBUTTON = 1003
)

// INITCOMMONCONTROLSEX structure
type INITCOMMONCONTROLSEX struct {
	DwSize uint32
	DwICC  uint32
}

// WNDCLASSEXW structure
type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

// MSG structure
type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// ProgressWindow represents a progress dialog window
type ProgressWindow struct {
	hwnd        syscall.Handle
	hwndStatus  syscall.Handle
	hwndProgress syscall.Handle
	hwndButton  syscall.Handle
	hInstance   syscall.Handle
	className   *uint16
	done        chan struct{}
	result      error
	mu          sync.Mutex
	isComplete  bool
	canClose    bool
}

var globalProgressWindow *ProgressWindow

func getModuleHandle() syscall.Handle {
	ret, _, _ := procGetModuleHandleW.Call(0)
	return syscall.Handle(ret)
}

func initCommonControls() {
	icex := INITCOMMONCONTROLSEX{
		DwSize: uint32(unsafe.Sizeof(INITCOMMONCONTROLSEX{})),
		DwICC:  ICC_PROGRESS_CLASS,
	}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icex)))
}

func utf16PtrFromString(s string) *uint16 {
	ptr, _ := syscall.UTF16PtrFromString(s)
	return ptr
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		controlID := int(wParam & 0xFFFF)
		notifyCode := int((wParam >> 16) & 0xFFFF)
		if controlID == IDC_CLOSEBUTTON && notifyCode == BN_CLICKED {
			if globalProgressWindow != nil && globalProgressWindow.canClose {
				procDestroyWindow.Call(uintptr(hwnd))
			}
		}
	case WM_CLOSE:
		if globalProgressWindow != nil && globalProgressWindow.canClose {
			procDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	case WM_UPDATE_PROGRESS:
		if globalProgressWindow != nil {
			procSendMessageW.Call(
				uintptr(globalProgressWindow.hwndProgress),
				PBM_SETPOS,
				wParam,
				0,
			)
		}
		return 0
	case WM_UPDATE_STATUS:
		if globalProgressWindow != nil && lParam != 0 {
			procSetWindowTextW.Call(
				uintptr(globalProgressWindow.hwndStatus),
				lParam,
			)
		}
		return 0
	case WM_ENABLE_CLOSE:
		if globalProgressWindow != nil {
			globalProgressWindow.canClose = true
			procEnableWindow.Call(uintptr(globalProgressWindow.hwndButton), 1)
		}
		return 0
	case WM_SET_COMPLETE:
		if globalProgressWindow != nil {
			globalProgressWindow.isComplete = true
			globalProgressWindow.canClose = true
			procEnableWindow.Call(uintptr(globalProgressWindow.hwndButton), 1)
			procSetWindowTextW.Call(
				uintptr(globalProgressWindow.hwndButton),
				uintptr(unsafe.Pointer(utf16PtrFromString("Close"))),
			)
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// getDPI returns the system DPI scale factor
func getDPI() int {
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return 96
	}
	defer procReleaseDC.Call(0, hdc)
	
	dpi, _, _ := procGetDeviceCaps.Call(hdc, LOGPIXELSY)
	if dpi == 0 {
		return 96
	}
	return int(dpi)
}

// scale scales a dimension by DPI
func scale(value int, dpi int) int {
	return value * dpi / 96
}

// NewProgressWindow creates and shows a new progress window
func NewProgressWindow(title string) *ProgressWindow {
	initCommonControls()

	pw := &ProgressWindow{
		hInstance: getModuleHandle(),
		done:      make(chan struct{}),
		canClose:  false,
	}
	globalProgressWindow = pw

	pw.className = utf16PtrFromString("BgStatusServiceProgressWindow")

	// Register window class
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     pw.hInstance,
		HbrBackground: syscall.Handle(6), // COLOR_WINDOW + 1
		LpszClassName: pw.className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Get DPI for proper scaling
	dpi := getDPI()
	
	// Window dimensions (scaled for DPI)
	windowWidth := scale(500, dpi)
	windowHeight := scale(200, dpi)
	padding := scale(20, dpi)
	statusHeight := scale(45, dpi) // Taller for multi-line status
	progressHeight := scale(22, dpi)
	buttonWidth := scale(100, dpi)
	buttonHeight := scale(30, dpi)

	// Create main window
	titlePtr := utf16PtrFromString(title)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(pw.className)),
		uintptr(unsafe.Pointer(titlePtr)),
		WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX,
		uintptr(CW_USEDEFAULT),
		uintptr(CW_USEDEFAULT),
		uintptr(windowWidth),
		uintptr(windowHeight),
		0, 0,
		uintptr(pw.hInstance),
		0,
	)
	pw.hwnd = syscall.Handle(hwnd)

	// Create status label (multi-line capable)
	staticClass := utf16PtrFromString("STATIC")
	initialStatus := utf16PtrFromString("Initializing...")
	statusHwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticClass)),
		uintptr(unsafe.Pointer(initialStatus)),
		WS_CHILD|WS_VISIBLE|SS_LEFT,
		uintptr(padding),
		uintptr(padding),
		uintptr(windowWidth-padding*2-scale(16, dpi)),
		uintptr(statusHeight),
		hwnd, IDC_STATUS,
		uintptr(pw.hInstance),
		0,
	)
	pw.hwndStatus = syscall.Handle(statusHwnd)

	// Create progress bar
	progressClass := utf16PtrFromString("msctls_progress32")
	progressHwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(progressClass)),
		0,
		WS_CHILD|WS_VISIBLE|PBS_SMOOTH,
		uintptr(padding),
		uintptr(padding+statusHeight+scale(10, dpi)),
		uintptr(windowWidth-padding*2-scale(16, dpi)),
		uintptr(progressHeight),
		hwnd, IDC_PROGRESS,
		uintptr(pw.hInstance),
		0,
	)
	pw.hwndProgress = syscall.Handle(progressHwnd)

	// Set progress range 0-100
	procSendMessageW.Call(uintptr(progressHwnd), PBM_SETRANGE32, 0, 100)

	// Create Close button (initially disabled)
	buttonClass := utf16PtrFromString("BUTTON")
	buttonText := utf16PtrFromString("Please wait...")
	buttonX := (windowWidth - buttonWidth) / 2
	buttonY := padding + statusHeight + scale(10, dpi) + progressHeight + scale(20, dpi)
	buttonHwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(buttonClass)),
		uintptr(unsafe.Pointer(buttonText)),
		WS_CHILD|WS_VISIBLE|BS_DEFPUSHBUTTON|WS_DISABLED,
		uintptr(buttonX),
		uintptr(buttonY),
		uintptr(buttonWidth),
		uintptr(buttonHeight),
		hwnd, IDC_CLOSEBUTTON,
		uintptr(pw.hInstance),
		0,
	)
	pw.hwndButton = syscall.Handle(buttonHwnd)

	// Show window
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	return pw
}

// SetProgress sets the progress bar value (0-100)
func (pw *ProgressWindow) SetProgress(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	procPostMessageW.Call(uintptr(pw.hwnd), WM_UPDATE_PROGRESS, uintptr(percent), 0)
}

// SetStatus sets the status text
func (pw *ProgressWindow) SetStatus(status string) {
	statusPtr := utf16PtrFromString(status)
	// We need to use SendMessage here to ensure the string is processed before the ptr becomes invalid
	procSendMessageW.Call(uintptr(pw.hwnd), WM_UPDATE_STATUS, 0, uintptr(unsafe.Pointer(statusPtr)))
}

// SetComplete marks the operation as complete and enables the close button
func (pw *ProgressWindow) SetComplete(success bool, message string) {
	pw.SetProgress(100)
	pw.SetStatus(message)
	procPostMessageW.Call(uintptr(pw.hwnd), WM_SET_COMPLETE, 0, 0)
}

// ProcessMessages processes pending window messages (call from main thread)
func (pw *ProgressWindow) ProcessMessages() bool {
	var msg MSG
	for {
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0, PM_REMOVE,
		)
		if ret == 0 {
			return true // No more messages, window still open
		}
		if msg.Message == WM_DESTROY || (msg.Message == WM_CLOSE && pw.canClose) {
			return false // Window closed
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// RunMessageLoop runs the message loop until the window is closed
func (pw *ProgressWindow) RunMessageLoop() {
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret == 0 || ret == 0xFFFFFFFF {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	globalProgressWindow = nil
}

// Close closes the progress window
func (pw *ProgressWindow) Close() {
	if pw.hwnd != 0 {
		procDestroyWindow.Call(uintptr(pw.hwnd))
	}
}

// ProgressStep represents a step in the installation process
type ProgressStep struct {
	Name    string
	Percent int
}

// InstallSteps defines the progress steps for installation
var InstallSteps = []ProgressStep{
	{"Checking existing installation...", 5},
	{"Stopping existing service...", 15},
	{"Removing old service...", 25},
	{"Downloading latest version...", 40},
	{"Installing service...", 70},
	{"Starting service...", 90},
	{"Complete!", 100},
}

// UninstallSteps defines the progress steps for uninstallation
var UninstallSteps = []ProgressStep{
	{"Stopping service...", 15},
	{"Removing service...", 35},
	{"Removing files...", 55},
	{"Cleaning registry...", 75},
	{"Complete!", 100},
}

