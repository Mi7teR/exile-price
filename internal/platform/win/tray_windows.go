//go:build windows

package win

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32               = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIcon   = shell32.NewProc("Shell_NotifyIconW")
	procShellExecute      = shell32.NewProc("ShellExecuteW")
	procLoadIcon          = user32.NewProc("LoadIconW")
	procLoadImage         = user32.NewProc("LoadImageW")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenu        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procSetForeground     = user32.NewProc("SetForegroundWindow")
	procRegisterWindowMsg = user32.NewProc("RegisterWindowMessageW")
	taskbarCreatedOnce    sync.Once
	taskbarCreatedMessage uintptr
)

const (
	wmTray         = 0x8000 + 2 // WM_APP+2: tray icon callback
	wmNull         = 0x0000
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	nimAdd         = 0
	nimDelete      = 2
	nifMessage     = 1
	nifIcon        = 2
	nifTip         = 4
	idiApplication = 32512
	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
	trayID         = 1
)

// notifyIconData is NOTIFYICONDATAW (Vista and later); Go's field alignment
// matches the C layout on amd64.
type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     uintptr
}

func taskbarCreated() uintptr {
	taskbarCreatedOnce.Do(func() {
		name, _ := windows.UTF16PtrFromString("TaskbarCreated")
		taskbarCreatedMessage, _, _ = procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(name)))
	})
	return taskbarCreatedMessage
}

func (l *Listener) addTray(hwnd uintptr) {
	if l.ev.TrayTip == "" {
		return
	}
	icon := appIcon()
	nid := notifyIconData{Wnd: hwnd, ID: trayID, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: icon}
	nid.Size = uint32(unsafe.Sizeof(nid))
	tip, _ := windows.UTF16FromString(l.ev.TrayTip)
	copy(nid.Tip[:len(nid.Tip)-1], tip)
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

// RemoveTray takes the icon off the tray; call it before exiting, or a dead
// icon stays until the mouse passes over it.
func (l *Listener) RemoveTray() {
	if l == nil || l.hwnd == 0 || l.ev.TrayTip == "" {
		return
	}
	nid := notifyIconData{Wnd: l.hwnd, ID: trayID}
	nid.Size = uint32(unsafe.Sizeof(nid))
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func (l *Listener) onTray(hwnd, lp uintptr) {
	switch lp & 0xFFFF {
	case wmLButtonUp:
		if l.ev.TrayClick != nil {
			go l.ev.TrayClick()
		}
	case wmRButtonUp:
		if len(l.ev.TrayMenu) == 0 {
			return
		}
		menu, _, _ := procCreatePopupMenu.Call()
		for i, it := range l.ev.TrayMenu {
			label, _ := windows.UTF16PtrFromString(it.Label)
			procAppendMenu.Call(menu, 0, uintptr(i+1), uintptr(unsafe.Pointer(label)))
		}
		x, y := CursorPos()
		// The menu closes on a click elsewhere only if its window is foreground.
		procSetForeground.Call(hwnd)
		cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmNoNotify|tpmReturnCmd, uintptr(x), uintptr(y), 0, hwnd, 0)
		procPostMessage.Call(hwnd, wmNull, 0, 0)
		procDestroyMenu.Call(menu)
		if cmd >= 1 && int(cmd) <= len(l.ev.TrayMenu) {
			go l.ev.TrayMenu[cmd-1].Do()
		}
	}
}

// OpenFile opens a file with its default app, e.g. the log in Notepad.
func OpenFile(path string) {
	verb, _ := windows.UTF16PtrFromString("open")
	p, _ := windows.UTF16PtrFromString(path)
	const swShowNormal = 1
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, swShowNormal)
}

// appIcon is the exe's own icon (resource 1, the one Gio uses for its
// windows) at the small icon size, or the stock application icon.
func appIcon() uintptr {
	const (
		imageIcon              = 1
		lrShared               = 0x8000
		smCxSmIcon, smCySmIcon = 49, 50
	)
	mod, _, _ := procGetModuleHandle.Call(0)
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if h, _, _ := procLoadImage.Call(mod, 1, imageIcon, cx, cy, lrShared); h != 0 {
		return h
	}
	h, _, _ := procLoadIcon.Call(0, idiApplication)
	return h
}
