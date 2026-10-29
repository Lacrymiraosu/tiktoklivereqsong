//go:build windows

package platform

import (
	"errors"
	"math"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	gdi32             = windows.NewLazySystemDLL("gdi32.dll")
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	pShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pPostMessage      = user32.NewProc("PostMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessage  = user32.NewProc("DispatchMessageW")
	pCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	pAppendMenu       = user32.NewProc("AppendMenuW")
	pSetMenuDefault   = user32.NewProc("SetMenuDefaultItem")
	pTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	pDestroyMenu      = user32.NewProc("DestroyMenu")
	pGetCursorPos     = user32.NewProc("GetCursorPos")
	pSetForeground    = user32.NewProc("SetForegroundWindow")
	pRegisterWinMsg   = user32.NewProc("RegisterWindowMessageW")
	pCreateIconInd    = user32.NewProc("CreateIconIndirect")
	pMessageBox       = user32.NewProc("MessageBoxW")
	pCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	pCreateBitmap     = gdi32.NewProc("CreateBitmap")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
	pGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	pAttachConsole    = kernel32.NewProc("AttachConsole")
)

const (
	wmNull        = 0x0000
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	wmTray        = 0x8000 + 1 // WM_APP + 1

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	mfString    = 0x0
	mfGrayed    = 0x1
	mfChecked   = 0x8
	mfSeparator = 0x800
)

type notifyIconData struct {
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
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type wndClassEx struct {
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

// TrayItem is one entry of the tray icon's right-click menu.
type TrayItem struct {
	Label     string
	Checked   bool
	Disabled  bool
	Default   bool // shown bold; also what a left click does
	Separator bool
	OnClick   func()
}

// Tray is the notification-area icon. There is one per process.
type Tray struct {
	OnOpen func()            // left click
	Items  func() []TrayItem // built fresh each time the menu opens

	mu      sync.Mutex
	hwnd    uintptr
	nid     notifyIconData
	tip     string
	menu    []TrayItem
	done    chan struct{}
	restart uint32 // "TaskbarCreated": Explorer restarted, add the icon again
}

var theTray *Tray

// Start creates the icon on its own OS thread. It returns once the icon is
// visible (or failed).
func (t *Tray) Start(tip string) error {
	if theTray != nil {
		return errors.New("tray already started")
	}
	theTray = t
	t.done = make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(t.done)
		if err := t.create(tip); err != nil {
			errc <- err
			return
		}
		errc <- nil
		var m msg
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	return <-errc
}

func (t *Tray) create(tip string) error {
	inst, _, _ := pGetModuleHandle.Call(0)
	cls, _ := windows.UTF16PtrFromString("osureqTray")
	wc := wndClassEx{LpfnWndProc: windows.NewCallback(trayProc), HInstance: inst, LpszClassName: cls}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return errors.New("RegisterClassEx: " + err.Error())
	}
	title, _ := windows.UTF16PtrFromString("osureq")
	hwnd, _, err := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		return errors.New("CreateWindowEx: " + err.Error())
	}
	name, _ := windows.UTF16PtrFromString("TaskbarCreated")
	restart, _, _ := pRegisterWinMsg.Call(uintptr(unsafe.Pointer(name)))

	t.mu.Lock()
	t.hwnd, t.restart, t.tip = hwnd, uint32(restart), tip
	t.nid = notifyIconData{HWnd: hwnd, UID: 1, UFlags: nifMessage | nifIcon | nifTip, UCallbackMessage: wmTray, HIcon: makeIcon()}
	t.nid.CbSize = uint32(unsafe.Sizeof(t.nid))
	copyTip(&t.nid.SzTip, tip)
	t.mu.Unlock()
	if r, _, err := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid))); r == 0 {
		return errors.New("Shell_NotifyIcon: " + err.Error())
	}
	return nil
}

func copyTip(dst *[128]uint16, s string) {
	u := windows.StringToUTF16(s)
	if len(u) > len(dst) {
		u = append(u[:len(dst)-1], 0)
	}
	*dst = [128]uint16{}
	copy(dst[:], u)
}

// SetTip changes the hover text (ignored when unchanged).
func (t *Tray) SetTip(tip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwnd == 0 || tip == t.tip {
		return
	}
	t.tip = tip
	copyTip(&t.nid.SzTip, tip)
	t.nid.UFlags = nifTip
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
	t.nid.UFlags = nifMessage | nifIcon | nifTip
}

// Stop removes the icon and ends its thread.
func (t *Tray) Stop() {
	t.mu.Lock()
	hwnd := t.hwnd
	t.mu.Unlock()
	if hwnd != 0 {
		pPostMessage.Call(hwnd, wmClose, 0, 0)
		<-t.done
	}
}

func trayProc(hwnd, m, wParam, lParam uintptr) uintptr {
	t := theTray
	switch uint32(m) {
	case wmTray:
		switch lParam & 0xffff {
		case wmLButtonUp, wmLButtonDbl:
			if t.OnOpen != nil {
				go t.OnOpen()
			}
		case wmRButtonUp, wmContextMenu:
			t.showMenu(hwnd)
		}
		return 0
	case wmCommand:
		id := int(wParam & 0xffff)
		t.mu.Lock()
		menu := t.menu
		t.mu.Unlock()
		if id >= 1 && id <= len(menu) && menu[id-1].OnClick != nil {
			go menu[id-1].OnClick()
		}
		return 0
	case wmDestroy:
		t.mu.Lock()
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
		t.hwnd = 0
		t.mu.Unlock()
		pPostQuitMessage.Call(0)
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	}
	if t != nil && t.restart != 0 && uint32(m) == t.restart {
		t.mu.Lock()
		pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
		t.mu.Unlock()
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wParam, lParam)
	return r
}

func (t *Tray) showMenu(hwnd uintptr) {
	var items []TrayItem
	if t.Items != nil {
		items = t.Items()
	}
	t.mu.Lock()
	t.menu = items
	t.mu.Unlock()
	h, _, _ := pCreatePopupMenu.Call()
	if h == 0 {
		return
	}
	defer pDestroyMenu.Call(h)
	for i, it := range items {
		if it.Separator {
			pAppendMenu.Call(h, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if it.Checked {
			flags |= mfChecked
		}
		if it.Disabled {
			flags |= mfGrayed
		}
		label, _ := windows.UTF16PtrFromString(it.Label)
		pAppendMenu.Call(h, flags, uintptr(i+1), uintptr(unsafe.Pointer(label)))
		if it.Default {
			pSetMenuDefault.Call(h, uintptr(i+1), 0)
		}
	}
	var pt struct{ X, Y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForeground.Call(hwnd) // so the menu closes when clicking elsewhere
	const tpmBottomAlign, tpmRightButton = 0x20, 0x2
	pTrackPopupMenu.Call(h, tpmBottomAlign|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	pPostMessage.Call(hwnd, wmNull, 0, 0)
}

// makeIcon draws the app logo (pink ball, white ring, play triangle) as a
// 32×32 icon, so the exe needs no icon resource.
func makeIcon() uintptr {
	const n = 32
	type bitmapInfoHeader struct {
		Size          uint32
		Width         int32
		Height        int32
		Planes        uint16
		BitCount      uint16
		Compression   uint32
		SizeImage     uint32
		XPelsPerMeter int32
		YPelsPerMeter int32
		ClrUsed       uint32
		ClrImportant  uint32
	}
	bi := bitmapInfoHeader{Width: n, Height: -n, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	color, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	px := unsafe.Slice((*[4]byte)(bits), n*n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r, g, b, a := logoPixel(float64(x)+0.5, float64(y)+0.5)
			px[y*n+x] = [4]byte{b, g, r, a}
		}
	}
	maskBits := make([]byte, n*n/8)
	mask, _, _ := pCreateBitmap.Call(n, n, 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	ii := struct {
		FIcon    int32
		XHotspot uint32
		YHotspot uint32
		HbmMask  uintptr
		HbmColor uintptr
	}{FIcon: 1, HbmMask: mask, HbmColor: color}
	icon, _, _ := pCreateIconInd.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(color)
	pDeleteObject.Call(mask)
	return icon
}

func logoPixel(x, y float64) (r, g, b, a uint8) {
	const c, outer, ring = 16.0, 15.0, 12.2
	d := math.Hypot(x-c, y-c)
	cover := clamp01(outer + 0.5 - d) // anti-aliased outer edge
	if cover == 0 {
		return 0, 0, 0, 0
	}
	// pink ball shaded from the top-left like the CSS radial gradient
	t := clamp01(math.Hypot(x-11, y-10) / 20)
	pr, pg, pb := mix(0xff, 0xa6, 0xd0, 0xd8, 0x38, 0x7d, t)
	white := clamp01(d - ring + 0.5) // the white ring
	if inTriangle(x, y) {
		white = 1
	}
	r = uint8(lerp(pr, 255, white))
	g = uint8(lerp(pg, 255, white))
	b = uint8(lerp(pb, 255, white))
	return r, g, b, uint8(255 * cover)
}

func inTriangle(x, y float64) bool {
	// play triangle pointing right: (13,10) (13,22) (23,16)
	if x < 13 || x > 23 {
		return false
	}
	half := 6 * (23 - x) / 10
	return y >= 16-half && y <= 16+half
}

func mix(r1, g1, b1, r2, g2, b2 float64, t float64) (float64, float64, float64) {
	return lerp(r1, r2, t), lerp(g1, g2, t), lerp(b1, b2, t)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// MessageBox shows a blocking Windows message box (used when there is no console).
func MessageBox(title, text string, isError bool) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	flags := uintptr(0x40) // MB_ICONINFORMATION
	if isError {
		flags = 0x10 // MB_ICONERROR
	}
	pMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
}

// HasConsole reports whether the process has a console window to print to.
func HasConsole() bool {
	r, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
	return r != 0
}

// AttachParentConsole lets a GUI build print to the terminal it was started
// from (handy when developing). It reports whether a console was attached.
func AttachParentConsole() bool {
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := pAttachConsole.Call(attachParentProcess); r == 0 {
		return false
	}
	out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	os.Stdout, os.Stderr = out, out
	return true
}
