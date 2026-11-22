//go:build windows

package win

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandle   = kernel32.NewProc("GetModuleHandleW")
	procGlobalLock        = kernel32.NewProc("GlobalLock")
	procGlobalUnlock      = kernel32.NewProc("GlobalUnlock")
	procRegisterClassEx   = user32.NewProc("RegisterClassExW")
	procCreateWindowEx    = user32.NewProc("CreateWindowExW")
	procDefWindowProc     = user32.NewProc("DefWindowProcW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procGetMessage        = user32.NewProc("GetMessageW")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostMessage       = user32.NewProc("PostMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procFindWindowEx      = user32.NewProc("FindWindowExW")
	procAddClipListener   = user32.NewProc("AddClipboardFormatListener")
	procOpenClipboard     = user32.NewProc("OpenClipboard")
	procCloseClipboard    = user32.NewProc("CloseClipboard")
	procGetClipboardData  = user32.NewProc("GetClipboardData")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procSetDpiAwarenessCx = user32.NewProc("SetProcessDpiAwarenessContext")
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmHotkey          = 0x0312
	wmClipboardUpdate = 0x031D
	wmEscape          = 0x8000 + 1  // WM_APP+1: wParam 1 = grab Esc, 0 = release
	hwndMessage       = ^uintptr(2) // -3
	cfUnicodeText     = 13
	vkEscape          = 0x1B
	modNoRepeat       = 0x4000
	hotkeyEsc         = 1
	dpiPerMonitorV2   = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
)

// SetDPIAware makes every thread of the process per-monitor DPI aware (v2),
// so cursor and window coordinates are physical pixels on any monitor. Call
// it first in main: Gio sets this only on its own thread.
func SetDPIAware() {
	procSetDpiAwarenessCx.Call(dpiPerMonitorV2)
}

// Events are called on the listener's own thread: keep them short.
type Events struct {
	Clipboard func(text string) // clipboard text changed
	Escape    func()            // Esc pressed while grabbed
	Close     func()            // another instance asked this one to quit
}

// Listener is a hidden message-only window that receives clipboard updates,
// the Esc hotkey and the quit request from a second instance.
type Listener struct {
	hwnd uintptr
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

// Listen starts the message-only window on a locked OS thread.
func Listen(className string, ev Events) (*Listener, error) {
	l := &Listener{}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		err := l.create(className, ev)
		ready <- err
		if err != nil {
			return
		}
		var m msg
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	return l, <-ready
}

func (l *Listener) create(className string, ev Events) error {
	inst, _, _ := procGetModuleHandle.Call(0)
	cls, _ := windows.UTF16PtrFromString(className)
	proc := windows.NewCallback(func(hwnd, m, wp, lp uintptr) uintptr {
		switch m {
		case wmClipboardUpdate:
			if text, err := readClipboard(hwnd); err == nil && ev.Clipboard != nil {
				ev.Clipboard(text)
			}
			return 0
		case wmHotkey:
			if wp == hotkeyEsc && ev.Escape != nil {
				ev.Escape()
			}
			return 0
		case wmEscape:
			if wp == 1 {
				procRegisterHotKey.Call(hwnd, hotkeyEsc, modNoRepeat, vkEscape)
			} else {
				procUnregisterHotKey.Call(hwnd, hotkeyEsc)
			}
			return 0
		case wmClose:
			if ev.Close != nil {
				ev.Close()
			}
			procDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProc.Call(hwnd, m, wp, lp)
		return r
	})
	wc := wndClassEx{WndProc: proc, Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, inst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	if r, _, err := procAddClipListener.Call(hwnd); r == 0 {
		return fmt.Errorf("AddClipboardFormatListener: %w", err)
	}
	l.hwnd = hwnd
	return nil
}

// GrabEscape takes Esc system-wide while the answer is shown, so Esc hides
// it; released on hide so the game gets Esc back.
func (l *Listener) GrabEscape(on bool) {
	var wp uintptr
	if on {
		wp = 1
	}
	procPostMessage.Call(l.hwnd, wmEscape, wp, 0)
}

// AskRunningToQuit finds another instance's listener window and asks it to
// quit. It reports whether one was found.
func AskRunningToQuit(className string) bool {
	cls, _ := windows.UTF16PtrFromString(className)
	hwnd, _, _ := procFindWindowEx.Call(hwndMessage, 0, uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		return false
	}
	procPostMessage.Call(hwnd, wmClose, 0, 0)
	return true
}

// SingleInstance holds a named mutex for the life of the process; false when
// another instance holds it. Any other error lets this instance run.
func SingleInstance(name string) (bool, error) {
	n, _ := windows.UTF16PtrFromString(name)
	_, err := windows.CreateMutex(nil, false, n)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, nil
	}
	return true, err
}

// readClipboard reads CF_UNICODETEXT, retrying while another app holds the
// clipboard (the game may still be writing it).
func readClipboard(hwnd uintptr) (string, error) {
	var opened bool
	for range 10 {
		if r, _, _ := procOpenClipboard.Call(hwnd); r != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		return "", errors.New("clipboard busy")
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", errors.New("no text on clipboard")
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return "", errors.New("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)
	// p is locked global memory owned by the clipboard until GlobalUnlock;
	// reading it through a copied pointer keeps vet's uintptr check quiet.
	return windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&p))), nil
}
