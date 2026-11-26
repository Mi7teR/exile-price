package win

import (
	"encoding/binary"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetExtendedTcpTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

// TCPClientExe is the executable of the process that holds the local end of
// a TCP connection from port from to port to, both on this machine: the
// browser that posted to the notice receiver.
func TCPClientExe(from, to uint16) (string, error) {
	const tcpTableOwnerPIDConnections = 4
	var size uint32
	var buf []byte
	for {
		var p uintptr
		if len(buf) > 0 {
			p = uintptr(unsafe.Pointer(&buf[0]))
		}
		r, _, _ := procGetExtendedTcpTable.Call(p, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tcpTableOwnerPIDConnections, 0)
		if r == 0 {
			break
		}
		if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			return "", windows.Errno(r)
		}
		buf = make([]byte, size)
	}
	if len(buf) < 4 {
		return "", errors.New("empty tcp table")
	}
	// MIB_TCPTABLE_OWNER_PID: a count, then rows of six uint32: state,
	// local addr, local port, remote addr, remote port, pid. Ports are in
	// network byte order in the low two bytes.
	port := func(row []byte, i int) uint16 { return binary.BigEndian.Uint16(row[i*4:]) }
	n := int(binary.LittleEndian.Uint32(buf))
	for i := 0; i < n && 4+(i+1)*24 <= len(buf); i++ {
		row := buf[4+i*24 : 4+(i+1)*24]
		if port(row, 2) == from && port(row, 4) == to {
			return exeOf(binary.LittleEndian.Uint32(row[20:]))
		}
	}
	return "", errors.New("no such connection")
}

func exeOf(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	b := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(b))
	if err := windows.QueryFullProcessImageName(h, 0, &b[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(b[:n]), nil
}
