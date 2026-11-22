//go:build windows

// Package win holds the Win32 calls the overlay needs, through x/sys/windows
// lazy procs: no cgo.
package win

import (
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
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	wsExAppWindow    = 0x00040000
	hwndTopmost      = ^uintptr(0) // -1
	swpNoSize        = 0x0001
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

// ShowAt shows the window with its top-left corner at x, y without activating it.
func ShowAt(hwnd uintptr, x, y int32) {
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoActivate|swpShowWindow)
	procShowWindow.Call(hwnd, swShowNoActivate)
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
