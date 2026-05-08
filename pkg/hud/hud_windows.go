//go:build windows

package hud

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kaicmurilo/tokalytics/pkg/providers"
)

// ── Win32 constants ────────────────────────────────────────────────────────

const (
	wsExTopmost    = 0x00000008
	wsExToolwindow = 0x00000080
	wsPopup        = 0x80000000
	wsVisible      = 0x10000000
	csDropshadow   = 0x00020000

	wmPaint     = 0x000F
	wmClose     = 0x0010
	wmDestroy   = 0x0002
	wmKillfocus = 0x0008
	wmEraseBkg  = 0x0014

	dtLeft       = 0x00000000
	dtRight      = 0x00000002
	dtSingleline = 0x00000020
	dtVcenter    = 0x00000004
	dtNoprefix   = 0x00000800
	dtEllipsis   = 0x00040000

	spiGetWorkArea = 0x0030

	fwNormal = 400
	fwBold   = 700

	// Layout constants (pixels)
	hudW  = 350
	rowH  = 22
	hdrH  = 28
	padL  = 12
	padR  = 12
	padT  = 10
	barW  = 88
	barH  = 8
	nameW = 110
)

// ── Win32 struct types ─────────────────────────────────────────────────────

// Matches WNDCLASSEXW layout on 64-bit Windows (80 bytes).
type wndClassExW struct {
	cbSize, style           uint32
	lpfnWndProc             uintptr
	cbClsExtra, cbWndExtra  int32
	hInstance, hIcon, hCursor, hbrBackground uintptr
	lpszMenuName, lpszClassName *uint16
	hIconSm                 uintptr
}

// Matches PAINTSTRUCT layout (68 bytes on 64-bit).
type paintStruct struct {
	hdc  uintptr
	_    [60]byte
}

// hudRect matches RECT (16 bytes).
type hudRect struct{ l, t, r, b int32 }

// ── Lazy-loaded DLLs & procs ───────────────────────────────────────────────

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dwmapi  = windows.NewLazySystemDLL("dwmapi.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procFillRect         = user32.NewProc("FillRect")
	procDrawTextW        = user32.NewProc("DrawTextW")
	procSetBkMode        = user32.NewProc("SetBkMode")
	procSetTextColor     = user32.NewProc("SetTextColor")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procLoadCursorW      = user32.NewProc("LoadCursorW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procSystemParamInfoW = user32.NewProc("SystemParametersInfoW")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procCreateFontW      = gdi32.NewProc("CreateFontW")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// ── Colors (Win32 COLORREF = 0x00BBGGRR) ──────────────────────────────────

func rgb(r, g, b byte) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

var (
	clrBg     = rgb(0x16, 0x16, 0x28)
	clrHdr    = rgb(0x1e, 0x1e, 0x38)
	clrAccent = rgb(0x82, 0xaa, 0xff)
	clrText   = rgb(0xe0, 0xe0, 0xe0)
	clrMuted  = rgb(0x70, 0x70, 0x90)
	clrGreen  = rgb(0x00, 0xd2, 0x6a)
	clrYellow = rgb(0xf7, 0xc9, 0x48)
	clrRed    = rgb(0xff, 0x50, 0x50)
	clrBarBg  = rgb(0x2d, 0x2d, 0x42)
	clrBorder = rgb(0x2a, 0x2a, 0x44)
	clrSep    = rgb(0x28, 0x28, 0x40)
	clrCost   = rgb(0xff, 0xcc, 0x55)
)

// ── Package state ──────────────────────────────────────────────────────────

const hudClassName = "TokalyticsHUD"

var (
	hudMu     sync.Mutex
	hudHwnd   uintptr
	hudData   map[string]*providers.Usage
	classOnce sync.Once
	cbProc    uintptr
	fontNorm  uintptr
	fontBold  uintptr
)

func init() {
	cbProc = windows.NewCallback(wndProc)
}

// ── Public API ─────────────────────────────────────────────────────────────

// ShowHUD opens the HUD window, or closes it if already visible.
func ShowHUD() {
	hudMu.Lock()
	hw := hudHwnd
	hudMu.Unlock()
	if hw != 0 {
		procPostMessageW.Call(hw, wmClose, 0, 0)
		return
	}
	go runHUD()
}

// ── Internal ───────────────────────────────────────────────────────────────

func runHUD() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	classOnce.Do(func() {
		registerClass()
		fontNorm = makeFont(fwNormal)
		fontBold = makeFont(fwBold)
	})

	usages := providers.GetUsages()
	hudMu.Lock()
	hudData = usages
	hudMu.Unlock()

	h := computeHeight(usages)

	// Position bottom-right above taskbar using work area
	var wa hudRect
	procSystemParamInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	x := wa.r - hudW - 12
	y := wa.b - h - 12

	cls, _ := windows.UTF16PtrFromString(hudClassName)
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExTopmost|wsExToolwindow,
		uintptr(unsafe.Pointer(cls)),
		0,
		wsPopup|wsVisible,
		uintptr(x), uintptr(y), uintptr(hudW), uintptr(h),
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return
	}

	// DWM: dark mode + rounded corners (Win11) + accent border (Win11)
	applyDWM(hwnd)

	hudMu.Lock()
	hudHwnd = hwnd
	hudMu.Unlock()

	// MSG is 48 bytes on 64-bit Windows; we just need a correctly-sized buffer.
	var m [48]byte
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m[0])), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m[0])))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m[0])))
	}

	hudMu.Lock()
	if hudHwnd == hwnd {
		hudHwnd = 0
	}
	hudMu.Unlock()
}

func applyDWM(hwnd uintptr) {
	dark := uint32(1)
	procDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4) // DWMWA_USE_IMMERSIVE_DARK_MODE
	corner := uint32(2)
	procDwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), 4) // DWMWA_WINDOW_CORNER_PREFERENCE (Win11)
	border := clrBorder
	procDwmSetWindowAttribute.Call(hwnd, 34, uintptr(unsafe.Pointer(&border)), 4) // DWMWA_BORDER_COLOR (Win11)
}

func registerClass() {
	inst, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW
	cls, _ := windows.UTF16PtrFromString(hudClassName)
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		style:         csDropshadow,
		lpfnWndProc:   cbProc,
		hInstance:     inst,
		hCursor:       cursor,
		lpszClassName: cls,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

func makeFont(weight int32) uintptr {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	height := int32(-14)
	h, _, _ := procCreateFontW.Call(
		uintptr(height), 0, 0, 0, uintptr(weight),
		0, 0, 0, 1, 0, 0, 5, 0, // charset=DEFAULT, quality=CLEARTYPE
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

func computeHeight(usages map[string]*providers.Usage) int32 {
	order := []string{"claude", "cursor", "gemini", "codex"}
	h := int32(padT)
	for _, id := range order {
		u, ok := usages[id]
		if !ok {
			continue
		}
		h += hdrH
		h += int32(len(u.Windows)) * rowH
		if id == "claude" && (u.TodayCostUSD > 0 || u.Last30CostUSD > 0) {
			h += rowH
		}
		h += 10 // separator gap
	}
	h += padT
	if h < 80 {
		h = 80
	}
	return h
}

// ── Window procedure ───────────────────────────────────────────────────────

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			paintHUD(hdc, hwnd)
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0

	case wmEraseBkg:
		return 1 // prevent default background erase flicker

	case wmKillfocus:
		procDestroyWindow.Call(hwnd)
		return 0

	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// ── Painting ───────────────────────────────────────────────────────────────

func paintHUD(hdc, hwnd uintptr) {
	var cr hudRect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))

	// Background
	fillR(hdc, clrBg, 0, 0, cr.r, cr.b)

	procSetBkMode.Call(hdc, 1) // TRANSPARENT

	hudMu.Lock()
	usages := hudData
	hudMu.Unlock()

	if len(usages) == 0 {
		procSelectObject.Call(hdc, fontNorm)
		setClr(hdc, clrMuted)
		drawStr(hdc, "Aguardando dados...", padL, padT, cr.r-padR, padT+rowH, dtLeft|dtSingleline|dtVcenter|dtNoprefix)
		return
	}

	order := []string{"claude", "cursor", "gemini", "codex"}
	y := int32(padT)

	for _, id := range order {
		u, ok := usages[id]
		if !ok {
			continue
		}

		// Provider header row
		fillR(hdc, clrHdr, 0, y, cr.r, y+hdrH)
		procSelectObject.Call(hdc, fontBold)
		setClr(hdc, clrAccent)
		plan := u.Plan
		if plan == "" {
			plan = "Pro"
		}
		drawStr(hdc, u.Name, padL, y, cr.r/2, y+hdrH, dtLeft|dtSingleline|dtVcenter|dtNoprefix)
		setClr(hdc, clrMuted)
		drawStr(hdc, plan, padL, y, cr.r-padR, y+hdrH, dtRight|dtSingleline|dtVcenter|dtNoprefix)
		y += hdrH

		// Window rows
		for _, w := range u.Windows {
			if isInfo(w) {
				procSelectObject.Call(hdc, fontNorm)
				setClr(hdc, clrMuted)
				drawStr(hdc, w.Name, padL+16, y, cr.r-padR, y+rowH, dtLeft|dtSingleline|dtVcenter|dtNoprefix|dtEllipsis)
			} else {
				drawBarRow(hdc, y, w, cr.r)
			}
			y += rowH
		}

		// Cost row (Claude only)
		if id == "claude" && (u.TodayCostUSD > 0 || u.Last30CostUSD > 0) {
			procSelectObject.Call(hdc, fontNorm)
			setClr(hdc, clrCost)
			s := fmt.Sprintf("$ %.2f hoje   ·   $ %.2f / 30d", u.TodayCostUSD, u.Last30CostUSD)
			drawStr(hdc, s, padL, y, cr.r-padR, y+rowH, dtLeft|dtSingleline|dtVcenter|dtNoprefix)
			y += rowH
		}

		// Separator
		fillR(hdc, clrSep, 0, y+4, cr.r, y+5)
		y += 10
	}
}

func drawBarRow(hdc uintptr, y int32, w providers.RateWindow, winW int32) {
	// Name
	procSelectObject.Call(hdc, fontNorm)
	setClr(hdc, clrText)
	drawStr(hdc, w.Name, padL, y, padL+nameW-4, y+rowH, dtLeft|dtSingleline|dtVcenter|dtNoprefix|dtEllipsis)

	// Bar background
	barX := int32(padL + nameW)
	barY := y + (rowH-barH)/2
	fillR(hdc, clrBarBg, barX, barY, barX+barW, barY+barH)

	// Bar fill
	filled := int32(float64(barW) * w.PctUsed / 100)
	if filled < 0 {
		filled = 0
	}
	if filled > barW {
		filled = barW
	}
	if filled > 0 {
		fillR(hdc, barColor(w.PctUsed), barX, barY, barX+filled, barY+barH)
	}

	// Percent text
	pctX := barX + barW + 6
	setClr(hdc, clrMuted)
	drawStr(hdc, fmt.Sprintf("%.0f%%", w.PctUsed), pctX, y, pctX+36, y+rowH, dtLeft|dtSingleline|dtVcenter|dtNoprefix)

	// Reset time
	if !w.ResetsAt.IsZero() {
		rt := "↺ " + countdown(w.ResetsAt)
		drawStr(hdc, rt, pctX+38, y, winW-padR, y+rowH, dtRight|dtSingleline|dtVcenter|dtNoprefix)
	}
}

// ── GDI helpers ────────────────────────────────────────────────────────────

func fillR(hdc, color uintptr, x1, y1, x2, y2 int32) {
	br, _, _ := procCreateSolidBrush.Call(color)
	r := hudRect{x1, y1, x2, y2}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), br)
	procDeleteObject.Call(br)
}

func setClr(hdc, color uintptr) {
	procSetTextColor.Call(hdc, color)
}

func drawStr(hdc uintptr, s string, x1, y1, x2, y2 int32, flags uint32) {
	r := hudRect{x1, y1, x2, y2}
	p, _ := windows.UTF16PtrFromString(s)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), uintptr(flags))
}

// ── Domain helpers ─────────────────────────────────────────────────────────

func isInfo(w providers.RateWindow) bool {
	return w.PctUsed == 0 && w.PctLeft >= 99 && w.ResetsAt.IsZero()
}

func barColor(pct float64) uintptr {
	switch {
	case pct >= 90:
		return clrRed
	case pct >= 70:
		return clrYellow
	default:
		return clrGreen
	}
}

func countdown(t time.Time) string {
	now := time.Now()
	if t.IsZero() || t.Before(now) {
		return "agora"
	}
	secs := int(t.Sub(now).Seconds())
	totalMins := (secs + 59) / 60
	if totalMins < 1 {
		totalMins = 1
	}
	days := totalMins / (24 * 60)
	hours := (totalMins / 60) % 24
	mins := totalMins % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", totalMins)
	}
}
