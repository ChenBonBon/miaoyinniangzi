package sourceruntime

import (
	"net/netip"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Winsock SO_ERROR (winsock2.h); x/sys/windows does not export this option.
const winsockSOError = 0x1007

var workerWinsockOnce sync.Once
var workerWinsockError error

func initializeWorkerWinsock() error {
	workerWinsockOnce.Do(func() {
		var data windows.WSAData
		if err := windows.WSAStartup(0x202, &data); err != nil {
			workerWinsockError = sandboxFailure("worker Winsock initialization", err)
			return
		}
		if data.Version != 0x202 {
			workerWinsockError = sandboxFailure("worker Winsock version")
		}
		// Keep this reference until process exit. Go's net package initializes before
		// main and caches startup errors; its poller is not used for the OS probe.
	})
	return workerWinsockError
}

type winsockFDSet struct {
	Count   uint32
	Sockets [64]windows.Handle
}
type winsockTimeval struct{ Seconds, Microseconds int32 }

// nativeTCPProbe bypasses Go net's cached init result, not the OS policy. It
// explicitly initializes Winsock and performs a bounded, nonblocking connect.
func nativeTCPProbe(address string) (bool, error) {
	if err := initializeWorkerWinsock(); err != nil {
		return false, err
	}
	target, err := netip.ParseAddrPort(address)
	if err != nil || !target.Addr().Is4() || target.Port() == 0 {
		return false, sandboxFailure("worker network probe address")
	}
	socket, err := windows.Socket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP)
	if err != nil {
		return false, err
	}
	defer windows.Closesocket(socket)
	library := windows.NewLazySystemDLL("ws2_32.dll")
	ioctl, selectProc := library.NewProc("ioctlsocket"), library.NewProc("select")
	if ioctl.Find() != nil || selectProc.Find() != nil {
		return false, sandboxFailure("worker network probe API")
	}
	var nonblocking uint32 = 1
	result, _, callErr := ioctl.Call(uintptr(socket), 0x8004667e, uintptr(unsafe.Pointer(&nonblocking))) // FIONBIO
	if int32(result) == -1 {
		return false, callErr
	}
	err = windows.Connect(socket, &windows.SockaddrInet4{Addr: target.Addr().As4(), Port: int(target.Port())})
	if err == nil {
		return true, nil
	}
	if err != windows.WSAEWOULDBLOCK && err != windows.WSAEINPROGRESS {
		return false, err
	}
	write := winsockFDSet{Count: 1}
	write.Sockets[0] = socket
	exception := write
	timeout := winsockTimeval{Microseconds: 500000}
	result, _, callErr = selectProc.Call(0, 0, uintptr(unsafe.Pointer(&write)), uintptr(unsafe.Pointer(&exception)), uintptr(unsafe.Pointer(&timeout)))
	if int32(result) == -1 {
		return false, callErr
	}
	if result == 0 {
		return false, sandboxFailure("worker network probe timeout")
	}
	code, err := windows.GetsockoptInt(socket, windows.SOL_SOCKET, winsockSOError)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, syscall.Errno(code)
	}
	return true, nil
}
