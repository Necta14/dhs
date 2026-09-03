//go:build windows

// Win32 bindings and the few helpers the interface needs. Only what is used: no wrapper library,
// no cgo, so the result stays a single portable .exe that needs nothing installed.
package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateWindow       = user32.NewProc("UpdateWindow")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessage        = user32.NewProc("PostMessageW")
	pSendMessage        = user32.NewProc("SendMessageW")
	pSetWindowText      = user32.NewProc("SetWindowTextW")
	pGetWindowText      = user32.NewProc("GetWindowTextW")
	pGetWindowTextLen   = user32.NewProc("GetWindowTextLengthW")
	pMoveWindow         = user32.NewProc("MoveWindow")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pLoadCursor         = user32.NewProc("LoadCursorW")
	pLoadIcon           = user32.NewProc("LoadIconW")
	pMessageBox         = user32.NewProc("MessageBoxW")
	pSystemParamsInfo   = user32.NewProc("SystemParametersInfoW")
	pEnableWindow       = user32.NewProc("EnableWindow")
	pSetFocus           = user32.NewProc("SetFocus")
	pGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
	pSetDpiAwareness    = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pCreateFontIndirect = gdi32.NewProc("CreateFontIndirectW")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
	pInitCommonControls = comctl32.NewProc("InitCommonControlsEx")
	pBrowseForFolder    = shell32.NewProc("SHBrowseForFolderW")
	pPathFromIDList     = shell32.NewProc("SHGetPathFromIDListW")
	pCoTaskMemFree      = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")

	// GDI, for the drawing in render.go
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pStretchBlt             = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pRectangle              = gdi32.NewProc("Rectangle")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkColor             = gdi32.NewProc("SetBkColor")
	pFillRect               = user32.NewProc("FillRect")
	pDrawText               = user32.NewProc("DrawTextW")

	// Window plumbing for a custom-painted window
	pBeginPaint      = user32.NewProc("BeginPaint")
	pEndPaint        = user32.NewProc("EndPaint")
	pInvalidateRect  = user32.NewProc("InvalidateRect")
	pGetDC           = user32.NewProc("GetDC")
	pReleaseDC       = user32.NewProc("ReleaseDC")
	pTrackMouseEvent = user32.NewProc("TrackMouseEvent")
	pSetCursorProc   = user32.NewProc("SetCursor")
	pCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	pAppendMenu      = user32.NewProc("AppendMenuW")
	pTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	pDestroyMenu     = user32.NewProc("DestroyMenu")
	pClientToScreen  = user32.NewProc("ClientToScreen")
	pSetWindowLong   = user32.NewProc("SetWindowLongPtrW")
	pGetWindowLong   = user32.NewProc("GetWindowLongPtrW")

	// Desktop Window Manager: a dark title bar and rounded window corners are most of what makes
	// a window look like it belongs on Windows 11.
	pDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

// Window messages and styles. Named rather than inlined, so the layout code reads like intent.
const (
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsGroup            = 0x00020000

	wsExClientEdge = 0x00000200

	bsPushButton     = 0x00000000
	bsDefPushButton  = 0x00000001
	bsAutoCheckBox   = 0x00000003
	esAutoHScroll    = 0x00000080
	esMultiline      = 0x00000004
	esReadOnly       = 0x00000800
	esPassword       = 0x00000020
	esWantReturn     = 0x00001000
	ssLeft           = 0x00000000
	cbsDropDownList  = 0x00000003
	pbsSmooth        = 0x00000001
	pbsMarquee       = 0x00000008
	tcsFixedWidth    = 0x00000400
	swShowNormal     = 1
	swHide           = 0
	swShow           = 5
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	svmDefaultButton = 0

	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmSetFont       = 0x0030
	wmCommand       = 0x0111
	wmNotify        = 0x004E
	wmClose         = 0x0010
	wmGetMinMaxInfo = 0x0024
	wmApp           = 0x8000

	emSetSel      = 0x00B1
	emReplaceSel  = 0x00C2
	emScrollCaret = 0x00B7
	cbAddString   = 0x0143
	cbSetCurSel   = 0x014E
	cbGetCurSel   = 0x0147
	bmGetCheck    = 0x00F0
	bmSetCheck    = 0x00F1
	pbmSetRange32 = 0x0406
	pbmSetPos     = 0x0402
	pbmSetMarquee = 0x040A

	srcCopy       = 0x00CC0020
	nullBrush     = 5
	nullPen       = 8
	psSolid       = 0
	transparentBk = 1

	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmMouseLeave     = 0x02A3
	wmSetCursor      = 0x0020
	wmDpiChanged     = 0x02E0
	wmSettingChange  = 0x001A
	wmCtlColorEdit   = 0x0133
	wmCtlColorStatic = 0x0138
	wmKeyDown        = 0x0100
	vkTab            = 0x09

	dwmUseDarkMode      = 20
	dwmCornerPreference = 33
	dwmCornerRound      = 2

	tcmInsertItem = 0x1200 + 62 // TCM_INSERTITEMW
	tcmGetCurSel  = 0x130B
	tcnSelChange  = -551

	mbOK          = 0x0000
	mbIconError   = 0x0010
	mbIconWarning = 0x0030
	mbIconInfo    = 0x0040
	mbYesNo       = 0x0004
	idYes         = 6

	iccTabClasses      = 0x00000008
	iccProgressClass   = 0x00000020
	iccStandardClasses = 0x00004000

	// Per-monitor DPI v2, so the window is sharp on a scaled display.
	dpiAwarePerMonitorV2 = ^uintptr(3) // (DPI_AWARENESS_CONTEXT)-4, in two's complement
)

type rect struct{ Left, Top, Right, Bottom int32 }

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background windows.Handle
	MenuName, ClassName                *uint16
	IconSm                             windows.Handle
}

type msg struct {
	Owner   windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type tcItem struct {
	Mask                   uint32
	State, StateMask       uint32
	Text                   *uint16
	TextMax, Image, LParam int32
}

type logFont struct {
	Height, Width                                        int32
	Escapement, Orientation, Weight                      int32
	Italic, Underline, StrikeOut                         byte
	CharSet, OutPrecision, ClipPrecision, Quality, Pitch byte
	FaceName                                             [32]uint16
}

type nonClientMetrics struct {
	Size                              uint32
	BorderWidth                       int32
	ScrollWidth, ScrollHeight         int32
	CaptionWidth, CaptionHeight       int32
	CaptionFont                       logFont
	SmCaptionWidth, SmCaptionHeight   int32
	SmCaptionFont                     logFont
	MenuWidth, MenuHeight             int32
	MenuFont, StatusFont, MessageFont logFont
	PaddedBorderWidth                 int32
}

type initCommonControlsEx struct {
	Size, ICC uint32
}

type browseInfo struct {
	Owner       windows.HWND
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return &[]uint16{0}[0]
	}
	return p
}

func moduleHandle() windows.Handle {
	h, _, _ := pGetModuleHandle.Call(0)
	return windows.Handle(h)
}

// systemFont returns the font Windows itself uses for dialogs, scaled to the window's DPI. Using
// it is the difference between looking native and looking like a 1998 utility.
func systemFont(dpi int32) windows.Handle {
	var m nonClientMetrics
	m.Size = uint32(unsafe.Sizeof(m))
	const spiGetNonClientMetrics = 0x0029
	ok, _, _ := pSystemParamsInfo.Call(spiGetNonClientMetrics, uintptr(m.Size), uintptr(unsafe.Pointer(&m)), 0)
	lf := m.MessageFont
	if ok == 0 {
		lf = logFont{Height: -12, Weight: 400}
		copy(lf.FaceName[:], utf16Slice("Segoe UI"))
	}
	if dpi != 96 && dpi != 0 {
		lf.Height = lf.Height * dpi / 96
	}
	h, _, _ := pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf)))
	return windows.Handle(h)
}

func utf16Slice(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}

func createWindow(class, text string, style, exStyle uint32, x, y, w, h int32, parent windows.HWND, id uintptr) windows.HWND {
	hwnd, _, _ := pCreateWindowEx.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), id, uintptr(moduleHandle()), 0)
	return windows.HWND(hwnd)
}

func setFont(hwnd windows.HWND, font windows.Handle) {
	pSendMessage.Call(uintptr(hwnd), wmSetFont, uintptr(font), 1)
}

func setText(hwnd windows.HWND, s string) {
	pSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(utf16(s))))
}

func getText(hwnd windows.HWND) string {
	n, _, _ := pGetWindowTextLen.Call(uintptr(hwnd))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return windows.UTF16ToString(buf)
}

func moveWindow(hwnd windows.HWND, x, y, w, h int32) {
	pMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

func showWindow(hwnd windows.HWND, show bool) {
	cmd := uintptr(swHide)
	if show {
		cmd = swShow
	}
	pShowWindow.Call(uintptr(hwnd), cmd)
}

func enable(hwnd windows.HWND, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pEnableWindow.Call(uintptr(hwnd), v)
}

func checked(hwnd windows.HWND) bool {
	r, _, _ := pSendMessage.Call(uintptr(hwnd), bmGetCheck, 0, 0)
	return r == 1
}

func clientRect(hwnd windows.HWND) rect {
	var r rect
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	return r
}

func dpiOf(hwnd windows.HWND) int32 {
	if err := pGetDpiForWindow.Find(); err != nil {
		return 96
	}
	d, _, _ := pGetDpiForWindow.Call(uintptr(hwnd))
	if d == 0 {
		return 96
	}
	return int32(d)
}

func messageBox(parent windows.HWND, text, title string, flags uint32) int {
	r, _, _ := pMessageBox.Call(uintptr(parent),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(unsafe.Pointer(utf16(title))),
		uintptr(flags))
	return int(r)
}

// pickFolder shows the shell folder picker and returns "" if the user cancelled.
func pickFolder(parent windows.HWND, title string) string {
	const bifNewDialogStyle = 0x0040
	const bifReturnOnlyFsDirs = 0x0001
	name := make([]uint16, windows.MAX_PATH)
	bi := browseInfo{
		Owner:       parent,
		DisplayName: &name[0],
		Title:       utf16(title),
		Flags:       bifNewDialogStyle | bifReturnOnlyFsDirs,
	}
	idl, _, _ := pBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if idl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(idl)
	path := make([]uint16, windows.MAX_PATH)
	ok, _, _ := pPathFromIDList.Call(idl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return ""
	}
	return windows.UTF16ToString(path)
}

// initControls registers the tab control and the progress bar. Without it, creating either simply
// fails and the window comes up empty.
func initControls() {
	icc := initCommonControlsEx{
		Size: uint32(unsafe.Sizeof(initCommonControlsEx{})),
		ICC:  iccTabClasses | iccProgressClass | iccStandardClasses,
	}
	pInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))
}

func makeDpiAware() {
	if err := pSetDpiAwareness.Find(); err == nil {
		pSetDpiAwareness.Call(dpiAwarePerMonitorV2)
	}
}

func newCallback(fn func(windows.HWND, uint32, uintptr, uintptr) uintptr) uintptr {
	return syscall.NewCallback(func(hwnd windows.HWND, m uint32, w, l uintptr) uintptr {
		return fn(hwnd, m, w, l)
	})
}
