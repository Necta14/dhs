//go:build windows

// The drawing layer.
//
// GDI has no antialiasing and GDI+ cannot be called from Go for anything that takes a float: the
// Microsoft x64 convention passes floats in XMM registers and Go's syscall puts every argument in
// an integer register, so GdipCreateFont and friends would receive rubbish. Rather than carry an
// assembly trampoline, this draws every shape into a surface twice the size with plain integer GDI
// and shrinks it with a halftone stretch. The rounded corners come out smooth, and the text is
// drawn afterwards at native size so ClearType stays crisp.
//
// Hence two passes over the same layout code: pass one emits shapes, pass two emits text.
package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const superSample = 2

type renderer struct {
	// one is the surface the window finally sees; two is where shapes are drawn, at twice the size
	oneDC, twoDC   windows.Handle
	oneBmp, twoBmp windows.Handle
	w, h           int32
	shapes         bool // which pass we are in
	pal            palette
}

func (r *renderer) resize(ref windows.Handle, w, h int32) {
	if w == r.w && h == r.h && r.oneDC != 0 {
		return
	}
	r.free()
	r.w, r.h = w, h
	if w <= 0 || h <= 0 {
		return
	}
	dc1, _, _ := pCreateCompatibleDC.Call(uintptr(ref))
	bm1, _, _ := pCreateCompatibleBitmap.Call(uintptr(ref), uintptr(w), uintptr(h))
	pSelectObject.Call(dc1, bm1)
	dc2, _, _ := pCreateCompatibleDC.Call(uintptr(ref))
	bm2, _, _ := pCreateCompatibleBitmap.Call(uintptr(ref), uintptr(w*superSample), uintptr(h*superSample))
	pSelectObject.Call(dc2, bm2)
	r.oneDC, r.oneBmp = windows.Handle(dc1), windows.Handle(bm1)
	r.twoDC, r.twoBmp = windows.Handle(dc2), windows.Handle(bm2)
}

func (r *renderer) free() {
	for _, h := range []windows.Handle{r.oneBmp, r.twoBmp} {
		if h != 0 {
			pDeleteObject.Call(uintptr(h))
		}
	}
	for _, h := range []windows.Handle{r.oneDC, r.twoDC} {
		if h != 0 {
			pDeleteDC.Call(uintptr(h))
		}
	}
	r.oneDC, r.twoDC, r.oneBmp, r.twoBmp = 0, 0, 0, 0
}

// begin clears the shape surface to the window colour and switches to the shape pass.
func (r *renderer) begin() {
	r.shapes = true
	r.fillRect(0, 0, r.w, r.h, r.pal.window)
}

// midway shrinks the shape surface onto the final one and switches to the text pass.
func (r *renderer) midway() {
	const stretchHalftone = 4
	pSetStretchBltMode.Call(uintptr(r.oneDC), stretchHalftone)
	pStretchBlt.Call(uintptr(r.oneDC), 0, 0, uintptr(r.w), uintptr(r.h),
		uintptr(r.twoDC), 0, 0, uintptr(r.w*superSample), uintptr(r.h*superSample), srcCopy)
	r.shapes = false
}

func (r *renderer) present(dst windows.Handle) {
	pBitBlt.Call(uintptr(dst), 0, 0, uintptr(r.w), uintptr(r.h), uintptr(r.oneDC), 0, 0, srcCopy)
}

// ---- shapes, pass one -------------------------------------------------------------------------

func (r *renderer) fillRect(x, y, w, h int32, c color) {
	if !r.shapes || w <= 0 || h <= 0 {
		return
	}
	s := int32(superSample)
	rc := rect{x * s, y * s, (x + w) * s, (y + h) * s}
	br, _, _ := pCreateSolidBrush.Call(uintptr(c))
	pFillRect.Call(uintptr(r.twoDC), uintptr(unsafe.Pointer(&rc)), br)
	pDeleteObject.Call(br)
}

// roundRect fills and optionally outlines a rounded rectangle. A radius of zero gives a plain one.
func (r *renderer) roundRect(x, y, w, h, radius int32, fill color, hasFill bool, stroke color, hasStroke bool) {
	if !r.shapes || w <= 0 || h <= 0 {
		return
	}
	s := int32(superSample)
	x, y, w, h, radius = x*s, y*s, w*s, h*s, radius*s

	var brush, pen uintptr
	if hasFill {
		brush, _, _ = pCreateSolidBrush.Call(uintptr(fill))
	} else {
		brush, _, _ = pGetStockObject.Call(nullBrush)
	}
	if hasStroke {
		pen, _, _ = pCreatePen.Call(psSolid, uintptr(s), uintptr(stroke))
	} else {
		pen, _, _ = pGetStockObject.Call(nullPen)
	}
	oldB, _, _ := pSelectObject.Call(uintptr(r.twoDC), brush)
	oldP, _, _ := pSelectObject.Call(uintptr(r.twoDC), pen)

	if radius <= 0 {
		pRectangle.Call(uintptr(r.twoDC), uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h))
	} else {
		pRoundRect.Call(uintptr(r.twoDC), uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h),
			uintptr(radius*2), uintptr(radius*2))
	}

	pSelectObject.Call(uintptr(r.twoDC), oldB)
	pSelectObject.Call(uintptr(r.twoDC), oldP)
	if hasFill {
		pDeleteObject.Call(brush)
	}
	if hasStroke {
		pDeleteObject.Call(pen)
	}
}

func (r *renderer) hline(x, y, w int32, c color) { r.fillRect(x, y, w, 1, c) }

// ---- text, pass two ---------------------------------------------------------------------------

const (
	dtLeft        = 0x0000
	dtCenter      = 0x0001
	dtRight       = 0x0002
	dtVCenter     = 0x0004
	dtSingleLine  = 0x0020
	dtWordBreak   = 0x0010
	dtEndEllipsis = 0x8000
	dtNoPrefix    = 0x0800
)

func (r *renderer) text(x, y, w, h int32, s string, font windows.Handle, c color, flags uint32) {
	if r.shapes || s == "" {
		return
	}
	rc := rect{x, y, x + w, y + h}
	old, _, _ := pSelectObject.Call(uintptr(r.oneDC), uintptr(font))
	pSetBkMode.Call(uintptr(r.oneDC), transparentBk)
	pSetTextColor.Call(uintptr(r.oneDC), uintptr(c))
	u := utf16Slice(s)
	pDrawText.Call(uintptr(r.oneDC), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
		uintptr(unsafe.Pointer(&rc)), uintptr(flags|dtNoPrefix))
	pSelectObject.Call(uintptr(r.oneDC), old)
}

// measure returns the width a string needs, so buttons can size themselves to their label.
func (r *renderer) measure(s string, font windows.Handle) int32 {
	if s == "" || font == 0 {
		return 0
	}
	// Layout runs before the first WM_PAINT, when there is no back buffer yet. Without a DC
	// DrawText measures nothing and hands back the probe rectangle, which used to make every
	// button as wide as the window. Borrow the screen DC in that case.
	dc := r.oneDC
	var borrowed uintptr
	if dc == 0 {
		borrowed, _, _ = pGetDC.Call(0)
		dc = windows.Handle(borrowed)
		if dc == 0 {
			return int32(len(s)) * 7
		}
	}
	rc := rect{0, 0, 4000, 200}
	old, _, _ := pSelectObject.Call(uintptr(dc), uintptr(font))
	u := utf16Slice(s)
	const dtCalcRect = 0x0400
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
		uintptr(unsafe.Pointer(&rc)), dtCalcRect|dtSingleLine|dtNoPrefix)
	pSelectObject.Call(uintptr(dc), old)
	if borrowed != 0 {
		pReleaseDC.Call(0, borrowed)
	}
	return rc.Right
}

// ---- fonts ------------------------------------------------------------------------------------

// uiFont builds a font at a point size scaled for the DPI. Segoe UI Variable is the Windows 11
// face; Segoe UI is the fallback, and both keep the interface looking like the system it runs on.
func uiFont(dpi, points int32, semibold bool, mono bool) windows.Handle {
	weight := int32(400)
	if semibold {
		weight = 600
	}
	names := []string{"Segoe UI Variable Text", "Segoe UI", "Tahoma"}
	if mono {
		names = []string{"Cascadia Mono", "Consolas", "Courier New"}
	}
	lf := logFont{
		Height:  -(points * dpi / 72),
		Weight:  weight,
		Quality: 5, // CLEARTYPE_QUALITY
	}
	for _, n := range names {
		lf.FaceName = [32]uint16{}
		copy(lf.FaceName[:], utf16Slice(n))
		if h, _, _ := pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf))); h != 0 {
			return windows.Handle(h)
		}
	}
	return 0
}
