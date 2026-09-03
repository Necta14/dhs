//go:build windows

// The palette and the metrics. The numbers come from libadwaita, so the Windows interface and the
// GNOME one are recognisably the same program: the same greys, the same accent, the same 12 pixel
// card radius and the same generous spacing.
package main

import "golang.org/x/sys/windows/registry"

type color uint32 // 0x00BBGGRR, the order GDI wants

func rgb(r, g, b uint8) color { return color(uint32(b)<<16 | uint32(g)<<8 | uint32(r)) }

// mix blends two colours, t in 0..100. Used for hover states and for tinting a selected row with
// the accent instead of hardcoding a second shade.
func mix(a, b color, t int32) color {
	ar, ag, ab := int32(a&0xFF), int32((a>>8)&0xFF), int32((a>>16)&0xFF)
	br, bg, bb := int32(b&0xFF), int32((b>>8)&0xFF), int32((b>>16)&0xFF)
	f := func(x, y int32) uint32 { return uint32(x + (y-x)*t/100) }
	return color(f(ab, bb)<<16 | f(ag, bg)<<8 | f(ar, br))
}

type palette struct {
	dark bool

	window   color
	header   color
	sidebar  color
	card     color
	border   color
	divider  color
	text     color
	dim      color
	accent   color
	onAccent color
	good     color
	bad      color
	field    color
	knob     color
	track    color
}

func lightPalette() palette {
	return palette{
		window:   rgb(0xFA, 0xFA, 0xFA),
		header:   rgb(0xEB, 0xEB, 0xEB),
		sidebar:  rgb(0xF2, 0xF2, 0xF2),
		card:     rgb(0xFF, 0xFF, 0xFF),
		border:   rgb(0xDC, 0xDC, 0xDC),
		divider:  rgb(0xE8, 0xE8, 0xE8),
		text:     rgb(0x2E, 0x30, 0x34),
		dim:      rgb(0x7A, 0x7E, 0x83),
		accent:   rgb(0x35, 0x84, 0xE4),
		onAccent: rgb(0xFF, 0xFF, 0xFF),
		good:     rgb(0x1F, 0x8A, 0x40),
		bad:      rgb(0xC0, 0x1C, 0x28),
		field:    rgb(0xFF, 0xFF, 0xFF),
		knob:     rgb(0xFF, 0xFF, 0xFF),
		track:    rgb(0xD4, 0xD4, 0xD4),
	}
}

func darkPalette() palette {
	return palette{
		dark:     true,
		window:   rgb(0x24, 0x24, 0x24),
		header:   rgb(0x30, 0x30, 0x30),
		sidebar:  rgb(0x2A, 0x2A, 0x2A),
		card:     rgb(0x30, 0x30, 0x30),
		border:   rgb(0x3D, 0x3D, 0x3D),
		divider:  rgb(0x3A, 0x3A, 0x3A),
		text:     rgb(0xF2, 0xF2, 0xF2),
		dim:      rgb(0x9A, 0x9A, 0x9A),
		accent:   rgb(0x35, 0x84, 0xE4),
		onAccent: rgb(0xFF, 0xFF, 0xFF),
		good:     rgb(0x57, 0xE3, 0x89),
		bad:      rgb(0xFF, 0x7B, 0x63),
		field:    rgb(0x1E, 0x1E, 0x1E),
		knob:     rgb(0xFF, 0xFF, 0xFF),
		track:    rgb(0x50, 0x50, 0x50),
	}
}

// systemPrefersDark reads the same switch the Settings app writes. If the key is missing, which is
// what happens on some LTSC images, light is the safe assumption.
func systemPrefersDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return v == 0
}

func currentPalette() palette {
	if systemPrefersDark() {
		return darkPalette()
	}
	return lightPalette()
}

// Metrics, in unscaled pixels. Everything on screen goes through app.scale, so the same numbers
// hold at 100% and at 200%.
const (
	mHeaderH   = 48
	mSidebarW  = 236
	mPad       = 18
	mCardR     = 12
	mRowH      = 58
	mRowPad    = 14
	mGap       = 18
	mNavH      = 38
	mNavR      = 8
	mBtnH      = 34
	mBtnR      = 7
	mFieldH    = 30
	mFieldR    = 6
	mSwitchW   = 42
	mSwitchH   = 24
	mGroupGap  = 8
	mTitleSize = 15
	mBodySize  = 13
	mSmallSize = 12
	mMonoSize  = 12
)
