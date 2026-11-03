package sourceruntime

import (
	"net"
	"testing"
	"unsafe"
)

func TestWindowsNativeNetworkProbeControl(t *testing.T) {
	if err := initializeWorkerWinsock(); err != nil {
		t.Fatal(err)
	}
	if err := initializeWorkerWinsock(); err != nil {
		t.Fatal(err)
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(winsockFDSet{}) != 520 || unsafe.Sizeof(winsockTimeval{}) != 8) {
		t.Fatal("Winsock probe ABI layout mismatch")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("control listener failed")
	}
	defer listener.Close()
	connected, err := winsockTCPReferenceProbe(listener.Addr().String())
	if err != nil || !connected {
		t.Fatal("native network control failed to connect to a live ordinary-process listener")
	}
	if connected, err = winsockTCPReferenceProbe("invalid-address"); err == nil || connected {
		t.Fatal("invalid probe address accepted")
	}
}
