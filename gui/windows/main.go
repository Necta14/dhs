//go:build windows

// dhs-gui.exe - the Windows interface for DHS.
//
// A separate process that drives dhs.exe through its --json output (decision D9). It holds no
// business rule of its own: every number and every warning on screen comes from the core.
//
// Win32, drawn by hand, no cgo and no toolkit, so the result is one portable .exe that needs
// nothing installed. The look follows libadwaita on purpose: the GNOME front end and this one
// should read as the same program, with the same greys, the same accent and the same cards.
//
//	go build -ldflags "-s -w -H windowsgui" -o dhs-gui.exe ./gui/windows
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// widget kinds
const (
	kNav = iota
	kRowSwitch
	kRowCombo
	kField // the rounded frame plus the real EDIT that lives inside it
	kButton
)

// button styles
const (
	btnNormal = iota
	btnPrimary
)

type widget struct {
	id      int
	kind    int
	page    int // -1: present on every page
	style   int
	title   string
	sub     string
	r       rect
	on      bool
	values  []string
	sel     int
	hover   bool
	down    bool
	enabled bool
	edit    windows.HWND
	mono    bool
}

// A card is decoration: a rounded group behind a run of rows, with a small caption above it.
type card struct {
	r      rect
	titleR rect
	title  string
}

const (
	wNavScan = iota + 1
	wNavBackup
	wNavRestore

	wScanDest
	wScanPick
	wScanSecrets
	wScanAll
	wScanLevel
	wScanGo

	wBackDest
	wBackPick
	wBackName
	wBackLevel
	wBackPass
	wBackPass2
	wBackPlain
	wBackSecrets
	wBackAll
	wBackGo

	wRestPkg
	wRestPick
	wRestPass
	wRestConflict
	wRestList
	wRestPlan
	wRestGo

	wOutput
)

const msgDone = wmApp + 1

type app struct {
	hwnd   windows.HWND
	dpi    int32
	pal    palette
	r      renderer
	page   int
	binary string

	fTitle, fBody, fBold, fSmall, fMono windows.Handle
	brField                             windows.Handle

	w     map[int]*widget
	order []int

	cards      []card
	statusRect rect

	hover, active int
	tracking      bool

	mu      sync.Mutex
	result  string
	failed  bool
	running bool
	status  string
}

var a = &app{w: map[int]*widget{}, status: "Ready."}

func (a *app) s(v int32) int32 { return v * a.dpi / 96 }

func main() {
	makeDpiAware()

	bin, err := findBinary()
	if err != nil {
		messageBox(0, err.Error()+"\n\nDHS is a command-line tool; this window only drives it.",
			"DHS", mbOK|mbIconError)
		os.Exit(1)
	}
	a.binary = bin
	a.pal = currentPalette()

	cursor, _, _ := pLoadCursor.Call(0, 32512)
	icon, _, _ := pLoadIcon.Call(uintptr(moduleHandle()), 2) // the group rsrc emits
	if icon == 0 {
		icon, _, _ = pLoadIcon.Call(0, 32512)
	}
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   newCallback(wndProc),
		Instance:  moduleHandle(),
		Cursor:    windows.Handle(cursor),
		Icon:      windows.Handle(icon),
		IconSm:    windows.Handle(icon),
		ClassName: utf16("DhsMainWindow"),
	}
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		messageBox(0, fmt.Sprintf("could not register the window class: %v", err), "DHS", mbOK|mbIconError)
		os.Exit(1)
	}

	hwnd := createWindow("DhsMainWindow", "DHS", wsOverlappedWindow, 0, 100, 80, 1020, 720, 0, 0)
	if hwnd == 0 {
		os.Exit(1)
	}
	a.hwnd = hwnd
	dwmTouches(hwnd, a.pal.dark)
	pShowWindow.Call(uintptr(hwnd), swShowNormal)
	pUpdateWindow.Call(uintptr(hwnd))

	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// dwmTouches asks the compositor for a dark title bar and rounded window corners. Without these
// two, a custom-drawn window still announces itself as an old application.
func dwmTouches(hwnd windows.HWND, dark bool) {
	v := int32(0)
	if dark {
		v = 1
	}
	pDwmSetWindowAttribute.Call(uintptr(hwnd), dwmUseDarkMode, uintptr(unsafe.Pointer(&v)), 4)
	corner := int32(dwmCornerRound)
	pDwmSetWindowAttribute.Call(uintptr(hwnd), dwmCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
}

func wndProc(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case 0x0001: // WM_CREATE
		a.hwnd = hwnd
		a.dpi = dpiOf(hwnd)
		makeFonts()
		build()
		selectPage(0)
		return 0

	case wmEraseBkgnd:
		return 1 // everything is painted in WM_PAINT, so no flicker

	case wmPaint:
		var ps [128]byte
		dc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps[0])))
		paint(windows.Handle(dc))
		pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps[0])))
		return 0

	case wmSize:
		layout()
		invalidate()
		return 0

	case wmMouseMove:
		if !a.tracking {
			track(hwnd)
		}
		setHover(hit(int32(int16(lParam&0xFFFF)), int32(int16((lParam>>16)&0xFFFF))))
		return 0

	case wmMouseLeave:
		a.tracking = false
		setHover(0)
		return 0

	case wmLButtonDown:
		if id := hit(int32(int16(lParam&0xFFFF)), int32(int16((lParam>>16)&0xFFFF))); id != 0 {
			a.active = id
			a.w[id].down = true
			invalidate()
		}
		return 0

	case wmLButtonUp:
		id := a.active
		a.active = 0
		if w := a.w[id]; w != nil {
			w.down = false
			invalidate()
			if hit(int32(int16(lParam&0xFFFF)), int32(int16((lParam>>16)&0xFFFF))) == id {
				activate(id)
			}
		}
		return 0

	case wmSetCursor:
		if a.hover != 0 {
			h, _, _ := pLoadCursor.Call(0, 32649) // IDC_HAND
			pSetCursorProc.Call(h)
			return 1
		}

	case wmCtlColorEdit, wmCtlColorStatic:
		pSetTextColor.Call(wParam, uintptr(a.pal.text))
		setBkColor(wParam, a.pal.field)
		if a.brField == 0 {
			b, _, _ := pCreateSolidBrush.Call(uintptr(a.pal.field))
			a.brField = windows.Handle(b)
		}
		return uintptr(a.brField)

	case wmSettingChange:
		if p := currentPalette(); p.dark != a.pal.dark {
			a.pal = p
			freeBrushes()
			dwmTouches(hwnd, p.dark)
			invalidate()
		}
		return 0

	case wmDpiChanged:
		a.dpi = int32(wParam & 0xFFFF)
		freeFonts()
		makeFonts()
		layout()
		invalidate()
		return 0

	case msgDone:
		a.mu.Lock()
		text, failed := a.result, a.failed
		a.running = false
		a.mu.Unlock()
		setText(a.w[wOutput].edit, text)
		if failed {
			a.status = "It did not work. The message is below."
		} else {
			a.status = "Done."
		}
		setBusy(false)
		invalidate()
		return 0

	case wmDestroy:
		freeFonts()
		freeBrushes()
		a.r.free()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func setBkColor(dc uintptr, c color) { pSetBkColor.Call(dc, uintptr(c)) }

func makeFonts() {
	a.fTitle = uiFont(a.dpi, mTitleSize, true, false)
	a.fBody = uiFont(a.dpi, mBodySize, false, false)
	a.fBold = uiFont(a.dpi, mBodySize, true, false)
	a.fSmall = uiFont(a.dpi, mSmallSize, false, false)
	a.fMono = uiFont(a.dpi, mMonoSize, false, true)
	for _, id := range a.order {
		if w := a.w[id]; w.edit != 0 {
			f := a.fBody
			if w.mono {
				f = a.fMono
			}
			setFont(w.edit, f)
		}
	}
}

func freeFonts() {
	for _, h := range []windows.Handle{a.fTitle, a.fBody, a.fBold, a.fSmall, a.fMono} {
		if h != 0 {
			pDeleteObject.Call(uintptr(h))
		}
	}
	a.fTitle, a.fBody, a.fBold, a.fSmall, a.fMono = 0, 0, 0, 0, 0
}

func freeBrushes() {
	if a.brField != 0 {
		pDeleteObject.Call(uintptr(a.brField))
		a.brField = 0
	}
}

func invalidate() { pInvalidateRect.Call(uintptr(a.hwnd), 0, 0) }

func track(hwnd windows.HWND) {
	type tme struct {
		Size, Flags uint32
		Track       windows.HWND
		Hover       uint32
	}
	t := tme{Flags: 0x0002 /*TME_LEAVE*/, Track: hwnd}
	t.Size = uint32(unsafe.Sizeof(t))
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
	a.tracking = true
}

// ---- building ---------------------------------------------------------------------------------

func addWidget(w *widget) {
	w.enabled = true
	a.w[w.id] = w
	a.order = append(a.order, w.id)
}

// field creates a borderless EDIT: the rounded frame around it is drawn, so it matches the cards
// instead of carrying the old Windows sunken border.
func field(id, page int, title string, password, output bool, initial string) {
	style := uint32(wsChild | wsTabStop | esAutoHScroll)
	if password {
		style |= esPassword
	}
	if output {
		style = wsChild | wsTabStop | esMultiline | esReadOnly | wsVScroll
	}
	h := createWindow("Edit", initial, style, 0, 0, 0, 0, 0, a.hwnd, uintptr(id))
	addWidget(&widget{id: id, kind: kField, page: page, title: title, edit: h, mono: output})
}

func build() {
	addWidget(&widget{id: wNavScan, kind: kNav, page: -1, title: "Scan", sub: "See what would be saved"})
	addWidget(&widget{id: wNavBackup, kind: kNav, page: -1, title: "Create a package", sub: "Write it to a drive"})
	addWidget(&widget{id: wNavRestore, kind: kNav, page: -1, title: "Restore", sub: "Put the files back"})

	levels := []string{"Compatible, opens anywhere", "Balanced", "Maximum"}

	field(wScanDest, 0, "Drive", false, false, "")
	addWidget(&widget{id: wScanPick, kind: kButton, page: 0, title: "Choose"})
	addWidget(&widget{id: wScanSecrets, kind: kRowSwitch, page: 0,
		title: "Include secrets", sub: "SSH and GPG keys, browser passwords, tokens"})
	addWidget(&widget{id: wScanAll, kind: kRowSwitch, page: 0,
		title: "Exclude nothing", sub: "Also caches, games and virtual machines"})
	addWidget(&widget{id: wScanLevel, kind: kRowCombo, page: 0, title: "Compression", values: levels, sel: 1})
	addWidget(&widget{id: wScanGo, kind: kButton, page: 0, title: "Scan", style: btnPrimary})

	field(wBackDest, 1, "Drive", false, false, "")
	addWidget(&widget{id: wBackPick, kind: kButton, page: 1, title: "Choose"})
	field(wBackName, 1, "Name", false, false, "migration")
	addWidget(&widget{id: wBackLevel, kind: kRowCombo, page: 1, title: "Compression", values: levels, sel: 1})
	field(wBackPass, 1, "Passphrase", true, false, "")
	field(wBackPass2, 1, "Again", true, false, "")
	addWidget(&widget{id: wBackPlain, kind: kRowSwitch, page: 1,
		title: "No encryption", sub: "Anyone who finds the drive can read everything"})
	addWidget(&widget{id: wBackSecrets, kind: kRowSwitch, page: 1,
		title: "Include secrets", sub: "SSH and GPG keys, browser passwords, tokens"})
	addWidget(&widget{id: wBackAll, kind: kRowSwitch, page: 1,
		title: "Exclude nothing", sub: "Also caches, games and virtual machines"})
	addWidget(&widget{id: wBackGo, kind: kButton, page: 1, title: "Create the package", style: btnPrimary})

	field(wRestPkg, 2, "Package", false, false, "")
	addWidget(&widget{id: wRestPick, kind: kButton, page: 2, title: "Choose"})
	field(wRestPass, 2, "Passphrase", true, false, "")
	addWidget(&widget{id: wRestConflict, kind: kRowCombo, page: 2, title: "If a file already exists",
		values: []string{"Keep both", "Skip it", "Overwrite"}, sel: 0})
	addWidget(&widget{id: wRestList, kind: kButton, page: 2, title: "What is inside"})
	addWidget(&widget{id: wRestPlan, kind: kButton, page: 2, title: "Show the plan", style: btnPrimary})
	addWidget(&widget{id: wRestGo, kind: kButton, page: 2, title: "Restore now"})

	field(wOutput, -1, "", false, true, welcome())
	makeFonts()
}

func welcome() string {
	return "DHS - Direct Handoff Suite\r\n\r\n" +
		"This window drives " + a.binary + ".\r\n" +
		"Everything below comes from that program; this interface decides nothing.\r\n\r\n" +
		"Start with Scan. It reads your profile and says how large the package would be and\r\n" +
		"whether it fits the drive you pick, without writing a single byte.\r\n\r\n" +
		"This is a pre-release. Do not migrate anything you cannot afford to lose yet.\r\n"
}

func selectPage(p int) {
	a.page = p
	for _, id := range a.order {
		w := a.w[id]
		if w.edit != 0 {
			showWindow(w.edit, w.page == -1 || w.page == p)
		}
	}
	layout()
	invalidate()
}

// ---- layout ----------------------------------------------------------------------------------

func layout() {
	if len(a.w) == 0 || a.hwnd == 0 {
		return
	}
	cr := clientRect(a.hwnd)
	s := a.s
	side, head := s(mSidebarW), s(mHeaderH)

	navH := s(mNavH + 16)
	navY := head + s(mPad)
	for i, id := range []int{wNavScan, wNavBackup, wNavRestore} {
		top := navY + int32(i)*(navH+s(4))
		a.w[id].r = rect{s(8), top, side - s(16), top + navH}
	}

	cx := side + s(mPad)
	cw := cr.Right - cx - s(mPad)
	if cw < s(320) {
		cw = s(320)
	}
	y := head + s(mPad)
	a.cards = a.cards[:0]

	// placeField positions the borderless EDIT inside its drawn frame.
	placeField := func(w *widget, x, top, width int32) {
		h := s(mFieldH)
		w.r = rect{x, top, x + width, top + h}
		pad := s(9)
		eh := s(mBodySize + 9)
		moveWindow(w.edit, x+pad, top+(h-eh)/2, width-2*pad, eh)
	}

	// group lays out one card: a caption, then one row per widget.
	group := func(title string, rows []int, trailing map[int]int) {
		y += s(mSmallSize + 14)
		top := y
		cardH := int32(len(rows)) * s(mRowH)
		for i, id := range rows {
			w := a.w[id]
			ry := top + int32(i)*s(mRowH)
			switch w.kind {
			case kField:
				trailW := int32(0)
				if t, ok := trailing[id]; ok {
					tb := a.w[t]
					bw := a.buttonWidth(tb)
					trailW = bw + s(10)
					tb.r = rect{cx + cw - s(mRowPad) - bw, ry + (s(mRowH)-s(mBtnH))/2,
						cx + cw - s(mRowPad), ry + (s(mRowH)-s(mBtnH))/2 + s(mBtnH)}
				}
				labelW := s(96)
				placeField(w, cx+s(mRowPad)+labelW, ry+(s(mRowH)-s(mFieldH))/2,
					cw-2*s(mRowPad)-labelW-trailW)
			default:
				w.r = rect{cx, ry, cx + cw, ry + s(mRowH)}
			}
		}
		a.cards = append(a.cards, card{
			r:      rect{cx, top, cx + cw, top + cardH},
			titleR: rect{cx, top - s(mSmallSize+14), cx + cw, top},
			title:  title,
		})
		y = top + cardH + s(mGroupGap)
	}

	buttonsRow := func(ids ...int) {
		y += s(10)
		bx := cx
		for _, id := range ids {
			w := a.w[id]
			bw := a.buttonWidth(w)
			w.r = rect{bx, y, bx + bw, y + s(mBtnH)}
			bx += bw + s(8)
		}
		y += s(mBtnH) + s(mGap)
	}

	switch a.page {
	case 0:
		group("DESTINATION", []int{wScanDest}, map[int]int{wScanDest: wScanPick})
		group("WHAT TO INCLUDE", []int{wScanSecrets, wScanAll}, nil)
		group("COMPRESSION", []int{wScanLevel}, nil)
		buttonsRow(wScanGo)
	case 1:
		group("DESTINATION", []int{wBackDest}, map[int]int{wBackDest: wBackPick})
		group("THE PACKAGE", []int{wBackName, wBackLevel}, nil)
		group("PASSPHRASE", []int{wBackPass, wBackPass2, wBackPlain}, nil)
		group("WHAT TO INCLUDE", []int{wBackSecrets, wBackAll}, nil)
		buttonsRow(wBackGo)
	case 2:
		group("THE PACKAGE", []int{wRestPkg}, map[int]int{wRestPkg: wRestPick})
		group("PASSPHRASE", []int{wRestPass}, nil)
		group("CONFLICTS", []int{wRestConflict}, nil)
		buttonsRow(wRestList, wRestPlan, wRestGo)
	}

	statusH := s(20)
	outTop := y
	outH := cr.Bottom - outTop - s(mPad) - statusH
	if outH < s(90) {
		outH = s(90)
	}
	ow := a.w[wOutput]
	ow.r = rect{cx, outTop, cx + cw, outTop + outH}
	pad := s(12)
	moveWindow(ow.edit, cx+pad, outTop+pad, cw-2*pad, outH-2*pad)
	a.statusRect = rect{cx + s(2), outTop + outH + s(3), cx + cw, outTop + outH + s(3) + statusH}
}

func (a *app) buttonWidth(w *widget) int32 {
	if w == nil {
		return 0
	}
	width := a.r.measure(w.title, a.fBody) + a.s(30)
	if min := a.s(96); width < min {
		width = min
	}
	return width
}

// ---- painting ---------------------------------------------------------------------------------

func paint(dc windows.Handle) {
	cr := clientRect(a.hwnd)
	a.r.pal = a.pal
	a.r.resize(dc, cr.Right, cr.Bottom)
	if a.r.oneDC == 0 {
		return
	}
	a.r.begin()
	draw(cr) // shapes
	a.r.midway()
	draw(cr) // text
	a.r.present(dc)
}

func draw(cr rect) {
	s, p, r := a.s, a.pal, &a.r
	head, side := s(mHeaderH), s(mSidebarW)

	r.fillRect(0, 0, cr.Right, head, p.header)
	r.fillRect(0, head, side, cr.Bottom-head, p.sidebar)
	r.hline(0, head-1, cr.Right, p.border)
	r.fillRect(side-1, head, 1, cr.Bottom-head, p.border)

	r.text(s(mPad), 0, side, head, "DHS", a.fTitle, p.text, dtLeft|dtVCenter|dtSingleLine)
	r.text(side+s(mPad), 0, cr.Right-side-2*s(mPad), head,
		[]string{"Scan", "Create a package", "Restore"}[a.page],
		a.fBold, p.dim, dtLeft|dtVCenter|dtSingleLine)

	for _, c := range a.cards {
		r.roundRect(c.r.Left, c.r.Top, c.r.Right-c.r.Left, c.r.Bottom-c.r.Top, s(mCardR),
			p.card, true, p.border, true)
		r.text(c.titleR.Left+s(4), c.titleR.Top, c.titleR.Right-c.titleR.Left,
			c.titleR.Bottom-c.titleR.Top, c.title, a.fSmall, p.dim, dtLeft|dtVCenter|dtSingleLine)
	}
	// hairlines between rows of the same card
	for _, c := range a.cards {
		rows := (c.r.Bottom - c.r.Top) / s(mRowH)
		for i := int32(1); i < rows; i++ {
			r.hline(c.r.Left+s(mRowPad), c.r.Top+i*s(mRowH), c.r.Right-c.r.Left-2*s(mRowPad), p.divider)
		}
	}

	for _, id := range a.order {
		w := a.w[id]
		if w.page != -1 && w.page != a.page {
			continue
		}
		drawWidget(r, w)
	}

	r.text(a.statusRect.Left, a.statusRect.Top, a.statusRect.Right-a.statusRect.Left,
		a.statusRect.Bottom-a.statusRect.Top, a.status, a.fSmall, p.dim, dtLeft|dtVCenter|dtSingleLine)
}

func drawWidget(r *renderer, w *widget) {
	s, p := a.s, a.pal
	rw, rh := w.r.Right-w.r.Left, w.r.Bottom-w.r.Top

	switch w.kind {
	case kNav:
		selected := w.id-wNavScan == a.page
		switch {
		case selected:
			r.roundRect(w.r.Left, w.r.Top, rw, rh, s(mNavR), mix(p.sidebar, p.accent, 16), true, 0, false)
		case w.hover:
			r.roundRect(w.r.Left, w.r.Top, rw, rh, s(mNavR), mix(p.sidebar, p.text, 6), true, 0, false)
		}
		col := p.text
		if selected {
			col = p.accent
		}
		r.text(w.r.Left+s(12), w.r.Top+s(7), rw-s(20), s(20), w.title, a.fBold, col,
			dtLeft|dtSingleLine|dtEndEllipsis)
		r.text(w.r.Left+s(12), w.r.Top+s(25), rw-s(20), s(21), w.sub, a.fSmall, p.dim,
			dtLeft|dtSingleLine|dtEndEllipsis)

	case kRowSwitch:
		tw, th := s(mSwitchW), s(mSwitchH)
		tx := w.r.Right - s(mRowPad) - tw
		ty := w.r.Top + (rh-th)/2
		bg := p.track
		if w.on {
			bg = p.accent
		}
		if w.hover {
			bg = mix(bg, p.text, 8)
		}
		r.roundRect(tx, ty, tw, th, th/2, bg, true, 0, false)
		kx := tx + s(3)
		if w.on {
			kx = tx + tw - th + s(3)
		}
		r.roundRect(kx, ty+s(3), th-s(6), th-s(6), (th-s(6))/2, p.knob, true, 0, false)
		textW := rw - tw - 3*s(mRowPad)
		r.text(w.r.Left+s(mRowPad), w.r.Top+s(9), textW, s(20), w.title, a.fBody, p.text,
			dtLeft|dtSingleLine|dtEndEllipsis)
		r.text(w.r.Left+s(mRowPad), w.r.Top+s(29), textW, s(21), w.sub, a.fSmall, p.dim,
			dtLeft|dtSingleLine|dtEndEllipsis)

	case kRowCombo:
		bw := s(230)
		bx := w.r.Right - s(mRowPad) - bw
		by := w.r.Top + (rh-s(mFieldH))/2
		r.text(w.r.Left+s(mRowPad), w.r.Top, rw-bw-3*s(mRowPad), rh, w.title, a.fBody, p.text,
			dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		bg := p.field
		if w.hover {
			bg = mix(bg, p.text, 6)
		}
		r.roundRect(bx, by, bw, s(mFieldH), s(mFieldR), bg, true, p.border, true)
		val := ""
		if w.sel >= 0 && w.sel < len(w.values) {
			val = w.values[w.sel]
		}
		r.text(bx+s(10), by, bw-s(32), s(mFieldH), val, a.fBody, p.text,
			dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		// a chevron, drawn as two short bars so no glyph is needed
		cxp, cyp := bx+bw-s(18), by+s(mFieldH)/2-s(1)
		r.fillRect(cxp, cyp, s(4), s(2), p.dim)
		r.fillRect(cxp+s(3), cyp+s(2), s(4), s(2), p.dim)
		r.fillRect(cxp+s(6), cyp, s(4), s(2), p.dim)

	case kField:
		if w.id == wOutput {
			r.roundRect(w.r.Left, w.r.Top, rw, rh, s(mCardR), p.field, true, p.border, true)
			return
		}
		r.text(w.r.Left-s(96), w.r.Top, s(90), rh, w.title, a.fBody, p.dim,
			dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		r.roundRect(w.r.Left, w.r.Top, rw, rh, s(mFieldR), p.field, true, p.border, true)

	case kButton:
		fill, txt, border := p.card, p.text, p.border
		if w.style == btnPrimary {
			fill, txt, border = p.accent, p.onAccent, p.accent
		}
		switch {
		case !w.enabled:
			fill = mix(fill, p.window, 55)
			txt = p.dim
			border = mix(border, p.window, 55)
		case w.down:
			fill = mix(fill, p.text, 18)
		case w.hover:
			fill = mix(fill, p.text, 8)
		}
		r.roundRect(w.r.Left, w.r.Top, rw, rh, s(mBtnR), fill, true, border, true)
		r.text(w.r.Left, w.r.Top, rw, rh, w.title, a.fBody, txt, dtCenter|dtVCenter|dtSingleLine)
	}
}

// ---- interaction ------------------------------------------------------------------------------

func hit(x, y int32) int {
	for i := len(a.order) - 1; i >= 0; i-- {
		w := a.w[a.order[i]]
		if w.page != -1 && w.page != a.page {
			continue
		}
		if !w.enabled {
			continue
		}
		switch w.kind {
		case kNav, kButton, kRowSwitch, kRowCombo:
		default:
			continue
		}
		if x >= w.r.Left && x < w.r.Right && y >= w.r.Top && y < w.r.Bottom {
			return w.id
		}
	}
	return 0
}

func setHover(id int) {
	if a.hover == id {
		return
	}
	if w := a.w[a.hover]; w != nil {
		w.hover = false
	}
	a.hover = id
	if w := a.w[id]; w != nil {
		w.hover = true
	}
	invalidate()
}

func setBusy(busy bool) {
	for _, id := range []int{wScanGo, wBackGo, wRestGo, wRestPlan, wRestList,
		wScanPick, wBackPick, wRestPick, wNavScan, wNavBackup, wNavRestore} {
		a.w[id].enabled = !busy
	}
	invalidate()
}

func activate(id int) {
	a.mu.Lock()
	busy := a.running
	a.mu.Unlock()
	if busy {
		return
	}
	w := a.w[id]

	switch w.kind {
	case kRowSwitch:
		w.on = !w.on
		invalidate()
		return
	case kRowCombo:
		popup(w)
		return
	}

	switch id {
	case wNavScan:
		selectPage(0)
	case wNavBackup:
		selectPage(1)
	case wNavRestore:
		selectPage(2)
	case wScanPick, wBackPick:
		if p := pickFolder(a.hwnd, "Where should the package go?"); p != "" {
			setText(a.w[wScanDest].edit, p)
			setText(a.w[wBackDest].edit, p)
		}
	case wRestPick:
		if p := pickFolder(a.hwnd, "Pick the package folder, the one ending in .dhs"); p != "" {
			setText(a.w[wRestPkg].edit, p)
		}
	case wScanGo:
		doScan()
	case wBackGo:
		doBackup()
	case wRestList:
		doList()
	case wRestPlan:
		doRestore(false)
	case wRestGo:
		doRestore(true)
	}
}

// popup shows a native menu under a combo row, so the list of choices is drawn by Windows itself.
func popup(w *widget) {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	const mfString, mfChecked = 0x0000, 0x0008
	for i, v := range w.values {
		flags := uintptr(mfString)
		if i == w.sel {
			flags |= mfChecked
		}
		pAppendMenu.Call(m, flags, uintptr(i+1), uintptr(unsafe.Pointer(utf16(v))))
	}
	pt := struct{ X, Y int32 }{
		X: w.r.Right - a.s(mRowPad) - a.s(230),
		Y: w.r.Top + (w.r.Bottom-w.r.Top+a.s(mFieldH))/2,
	}
	pClientToScreen.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(&pt)))
	const tpmReturnCmd = 0x0100
	r, _, _ := pTrackPopupMenu.Call(m, tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(a.hwnd), 0)
	if r > 0 {
		w.sel = int(r) - 1
		invalidate()
	}
}

func warn(s string) { messageBox(a.hwnd, s, "DHS", mbOK|mbIconWarning) }

func levelArg(id int) string {
	switch a.w[id].sel {
	case 0:
		return "1"
	case 2:
		return "3"
	default:
		return "2"
	}
}

func doScan() {
	dest := strings.TrimSpace(getText(a.w[wScanDest].edit))
	args := []string{"scan", "--json", "--level", levelArg(wScanLevel)}
	if dest != "" {
		args = append(args, "--dest", dest)
	}
	if a.w[wScanSecrets].on {
		args = append(args, "--secrets")
	}
	if a.w[wScanAll].on {
		args = append(args, "--all")
	}
	start("Reading your profile...", args, func(out []byte) (string, error) {
		v, err := parse[scanOut](out)
		if err != nil {
			return "", err
		}
		return formatScan(v), nil
	})
}

func doBackup() {
	dest := strings.TrimSpace(getText(a.w[wBackDest].edit))
	if dest == "" {
		warn("Pick the drive that will hold the package.")
		return
	}
	name := strings.TrimSpace(getText(a.w[wBackName].edit))
	plain := a.w[wBackPlain].on
	p1 := getText(a.w[wBackPass].edit)
	p2 := getText(a.w[wBackPass2].edit)

	if !plain {
		if len(p1) < 8 {
			warn("The passphrase needs at least 8 characters.")
			return
		}
		if p1 != p2 {
			warn("The two passphrases are not the same.")
			return
		}
	} else if messageBox(a.hwnd,
		"Without encryption, anyone who picks up the drive can read every file on it, "+
			"including anything private in your profile.\n\nGo on anyway?",
		"DHS", mbYesNo|mbIconWarning) != idYes {
		return
	}

	args := []string{"backup", "--json", "--dest", dest, "--level", levelArg(wBackLevel), "--yes", "--verify"}
	if name != "" {
		args = append(args, "--name", name)
	}
	if a.w[wBackSecrets].on {
		args = append(args, "--secrets")
	}
	if a.w[wBackAll].on {
		args = append(args, "--all")
	}
	cleanup := func() {}
	if plain {
		args = append(args, "--no-encrypt")
	} else {
		pf, c, err := passFile(p1)
		if err != nil {
			warn(err.Error())
			return
		}
		cleanup = c
		args = append(args, "--passphrase-file", pf)
	}
	startCleanup("Packing. This can take a while; the window stays responsive.", args, cleanup,
		func(out []byte) (string, error) {
			v, err := parse[backupOut](out)
			if err != nil {
				return "", err
			}
			return formatBackup(v), nil
		})
}

func doList() {
	pkg := strings.TrimSpace(getText(a.w[wRestPkg].edit))
	if pkg == "" {
		warn("Pick the package folder first.")
		return
	}
	pf, cleanup, err := passFile(getText(a.w[wRestPass].edit))
	if err != nil {
		warn(err.Error())
		return
	}
	args := []string{"list", pkg, "--json", "--all"}
	if pf != "" {
		args = append(args, "--passphrase-file", pf)
	}
	startCleanup("Opening the package...", args, cleanup, func(out []byte) (string, error) {
		v, err := parse[listOut](out)
		if err != nil {
			return "", err
		}
		return formatList(v), nil
	})
}

func doRestore(write bool) {
	pkg := strings.TrimSpace(getText(a.w[wRestPkg].edit))
	if pkg == "" {
		warn("Pick the package folder first.")
		return
	}
	if write && messageBox(a.hwnd,
		"About to write files into your profile from:\n\n"+filepath.Base(pkg)+
			"\n\nExisting files are handled the way the Conflicts setting says. Go on?",
		"DHS", mbYesNo|mbIconWarning) != idYes {
		return
	}
	pf, cleanup, err := passFile(getText(a.w[wRestPass].edit))
	if err != nil {
		warn(err.Error())
		return
	}
	policy := []string{"keep-both", "skip", "overwrite"}[a.w[wRestConflict].sel]
	args := []string{"restore", pkg, "--json", "--conflicts", policy}
	if pf != "" {
		args = append(args, "--passphrase-file", pf)
	}
	if write {
		args = append(args, "--yes")
	} else {
		args = append(args, "--dry-run")
	}
	msg := "Working out the plan..."
	if write {
		msg = "Putting the files back..."
	}
	startCleanup(msg, args, cleanup, func(out []byte) (string, error) {
		v, err := parse[restoreOut](out)
		if err != nil {
			return "", err
		}
		return formatPlan(v, write), nil
	})
}

// passFile writes the passphrase to a file, because passing it on a command line would put it in
// the process list for anything on the machine to read.
func passFile(pass string) (string, func(), error) {
	if pass == "" {
		return "", func() {}, nil
	}
	f, err := os.CreateTemp("", "dhs-*.txt")
	if err != nil {
		return "", func() {}, fmt.Errorf("could not write the passphrase to a temporary file: %v", err)
	}
	name := f.Name()
	if _, err := f.WriteString(pass + "\n"); err != nil {
		f.Close()
		os.Remove(name)
		return "", func() {}, err
	}
	f.Close()
	return name, func() { os.Remove(name) }, nil
}

func start(status string, args []string, render func([]byte) (string, error)) {
	startCleanup(status, args, func() {}, render)
}

// startCleanup runs the binary off the UI thread and posts the result back, so the window never
// freezes while a package is being written.
func startCleanup(status string, args []string, cleanup func(), render func([]byte) (string, error)) {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()
	setBusy(true)
	a.status = status
	setText(a.w[wOutput].edit, "$ dhs "+strings.Join(args, " ")+"\r\n\r\nWorking...\r\n")
	invalidate()

	go func() {
		defer cleanup()
		out, err := run(a.binary, args...)
		text, failed := "", false
		if err != nil {
			failed = true
			text = "It did not work.\r\n\r\n" + strings.ReplaceAll(err.Error(), "\n", "\r\n")
			if len(out) > 0 {
				text += "\r\n\r\n" + string(out)
			}
		} else if s, rerr := render(out); rerr != nil {
			failed = true
			text = "The output could not be read: " + rerr.Error() + "\r\n\r\n" + string(out)
		} else {
			text = s
		}
		a.mu.Lock()
		a.result, a.failed = text, failed
		a.mu.Unlock()
		pPostMessage.Call(uintptr(a.hwnd), msgDone, 0, 0)
	}()
}
