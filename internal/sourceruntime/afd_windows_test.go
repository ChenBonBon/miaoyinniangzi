package sourceruntime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type afdControlResult struct {
	Connected bool
	Failure   string
}

func windowsAFDControlMain() int {
	if len(os.Args) != 3 {
		return 2
	}
	port, err := strconv.Atoi(os.Args[2])
	if err != nil || port < 1 || port > 65535 {
		return 2
	}
	connected, err := nativeTCPProbe(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	result := afdControlResult{Connected: connected}
	if err != nil {
		result.Failure = sandboxFailure("AFD ordinary control phase="+afdFailurePhase(err), err).Error()
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return 3
	}
	return 0
}
func requireNativeAFDControl(t *testing.T) {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal("AFD control listener failed")
	}
	defer listener.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("AFD control executable unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--miaoyin-afd-control-test", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal("AFD control pipe failed")
	}
	if command.Start() != nil {
		t.Fatal("AFD control child failed to start")
	}
	var result afdControlResult
	readErr := json.NewDecoder(io.LimitReader(output, 4096)).Decode(&result)
	waitErr := command.Wait()
	if readErr != nil || waitErr != nil {
		t.Fatalf("AFD ordinary control failed: read=%s wait=%s deadline=%t", windowsProbeErrorKind(readErr), windowsProbeErrorKind(waitErr), ctx.Err() != nil)
	}
	if result.Failure != "" {
		t.Fatal(result.Failure)
	}
	if !result.Connected {
		t.Fatal("AFD control did not connect")
	}
	listener.SetDeadline(time.Now().Add(time.Second))
	connection, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal("AFD control claimed success without an accepted connection")
	}
	connection.Close()
	t.Log("AFD ordinary control: actual loopback connection accepted; private ABI usable")
}
func TestWindowsAFDNetworkProbeControl(t *testing.T) { requireNativeAFDControl(t) }
func TestWindowsAFDProbeABIAndDenial(t *testing.T) {
	if unsafe.Sizeof(afdSockaddr{}) != 16 || unsafe.Sizeof(afdBindInfo{}) != 20 || unsafe.Sizeof(afdConnectInfo{}) != 40 || unsafe.Offsetof(afdConnectInfo{}.Address) != 24 {
		t.Fatal("AFD ABI mismatch")
	}
	packet := afdOpenEA()
	if packet[5] != 15 || binary.LittleEndian.Uint16(packet[6:8]) != 28 || string(packet[8:24]) != "AfdOpenPacketXX\x00" || binary.LittleEndian.Uint32(packet[24:28]) != afdIgnoreTDI || binary.LittleEndian.Uint32(packet[32:36]) != 2 || binary.LittleEndian.Uint32(packet[36:40]) != 1 || binary.LittleEndian.Uint32(packet[40:44]) != 6 {
		t.Fatal("AFD open EA layout mismatch")
	}
	for _, err := range []error{nil, syscall.Errno(10013), syscall.Errno(10093), syscall.Errno(10107), syscall.Errno(5), windows.NTStatus(0xc000000d), windows.STATUS_PENDING, errors.New("timeout")} {
		if networkPolicyDenied(err) {
			t.Fatal("non-policy result accepted as isolation")
		}
	}
	for _, code := range []uint32{0xc0000022, 0xc00000ca} {
		if !networkPolicyDenied(windows.NTStatus(code)) || !networkPolicyDenied(&afdProbeError{"connect", windows.NTStatus(code)}) {
			t.Fatal("kernel denial not recognized")
		}
	}
	if connected, err := nativeTCPProbe("invalid-address"); err == nil || connected {
		t.Fatal("invalid target accepted")
	}
	if sandboxFailure("AFD", windows.NTStatus(0xc0000022)).Error() != "Windows source sandbox: AFD (ntstatus=0xc0000022)" {
		t.Fatal("unsafe NTSTATUS diagnostic")
	}
}
