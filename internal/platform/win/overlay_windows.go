//go:build windows

// Package win holds the Win32 calls the overlay needs, through x/sys/windows
// lazy procs: no cgo.
package win

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procShowWindow       = user32.NewProc("ShowWindow")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetForeground    = user32.NewProc("GetForegroundWindow")
	procGetWindowText    = user32.NewProc("GetWindowTextW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	procMessageBeep      = user32.NewProc("MessageBeep")
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	wsExAppWindow    = 0x00040000
	hwndTopmost      = ^uintptr(0) // -1
	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	swpHideWindow    = 0x0080
	swpFrameChanged  = 0x0020
	swShowNoActivate = 4
	swHide           = 0
)

// MakeOverlay turns a window into a non-activating, topmost tool window with
// no taskbar button.
func MakeOverlay(hwnd uintptr) {
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	ex = ex&^wsExAppWindow | wsExTopmost | wsExToolWindow | wsExNoActivate
	procSetWindowLongPtr.Call(hwnd, gwlExStyle, ex)
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoSize|swpNoActivate|swpFrameChanged)
}

type rect struct{ Left, Top, Right, Bottom int32 }

// ShowAt shows the window near x, y without activating it: below-right of
// the point, flipped and clamped to stay inside that monitor's work area.
func ShowAt(hwnd uintptr, x, y int32) {
	var wr rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	w, h := wr.Right-wr.Left, wr.Bottom-wr.Top
	work := workArea(x, y)
	px, py := x+16, y+16
	if px+w > work.Right {
		px = x - 16 - w
	}
	if py+h > work.Bottom {
		py = y - 16 - h
	}
	px = max(work.Left, min(px, work.Right-w))
	py = max(work.Top, min(py, work.Bottom-h))
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(px), uintptr(py), 0, 0, swpNoSize|swpNoActivate|swpShowWindow)
	procShowWindow.Call(hwnd, swShowNoActivate)
}

// ShowTopRight shows the window in the top right corner of the work area of
// the monitor under the cursor, without activating it.
func ShowTopRight(hwnd uintptr) {
	var wr rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	work := workArea(CursorPos())
	px, py := work.Right-(wr.Right-wr.Left)-16, work.Top+16
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(px), uintptr(py), 0, 0, swpNoSize|swpNoActivate|swpShowWindow)
	procShowWindow.Call(hwnd, swShowNoActivate)
}

// Beep plays the system "information" sound.
func Beep() {
	const mbIconAsterisk = 0x40
	procMessageBeep.Call(mbIconAsterisk)
}

// Resize sets the window size in pixels without moving or activating it.
func Resize(hwnd uintptr, w, h int32) {
	procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(w), uintptr(h), swpNoMove|swpNoZOrder|swpNoActivate)
}

func workArea(x, y int32) rect {
	const monitorDefaultToNearest = 2
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32 // POINT by value on amd64
	mon, _, _ := procMonitorFromPoint.Call(pt, monitorDefaultToNearest)
	var mi struct {
		Size          uint32
		Monitor, Work rect
		Flags         uint32
	}
	mi.Size = uint32(unsafe.Sizeof(mi))
	procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi.Work
}

func Hide(hwnd uintptr) {
	procShowWindow.Call(hwnd, swHide)
}

// CursorPos returns the cursor position in screen pixels.
func CursorPos() (x, y int32) {
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt.X, pt.Y
}

// Foreground returns the foreground window and its title.
func Foreground() (uintptr, string) {
	h, _, _ := procGetForeground.Call()
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return h, windows.UTF16ToString(buf[:n])
}

// GiveBackFocus waits up to a second for hwnd to become the foreground
// window and, if it does, hands the foreground back to prev. do runs a
// function on hwnd's thread.
func GiveBackFocus(hwnd, prev uintptr, do func(func())) {
	if prev == 0 || prev == hwnd {
		return
	}
	for range 20 {
		if h, _, _ := procGetForeground.Call(); h == hwnd {
			do(func() { procSetForeground.Call(prev) })
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
