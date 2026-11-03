package sourceruntime

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AFD's transport-independent IPv4/TCP ABI. No Winsock catalog, capabilities,
// inherited network handles, DNS or user configuration is needed. Private ABI:
// native positive-control acceptance is mandatory on each supported Windows.
// References: System Informer phnt/ntafd.h, AFD_OPEN_PACKET / *_INFO_TL.
const (
	afdIgnoreTDI = 0x01000000
	afdBind      = 0x12003
	afdConnect   = 0x12007
)

type afdSockaddr struct {
	Family  uint16
	Port    [2]byte
	Address [4]byte
	Zero    [8]byte
}
type afdBindInfo struct {
	Share   uint32
	Address afdSockaddr
}
type afdConnectInfo struct {
	SanActive                     uint8
	Reserved                      [7]byte
	RootEndpoint, ConnectEndpoint windows.Handle
	Address                       afdSockaddr
}

type afdProbeError struct {
	operation string
	cause     error
}

func (err *afdProbeError) Error() string { return "AFD " + err.operation + " failed" }
func (err *afdProbeError) Unwrap() error { return err.cause }
func afdFailurePhase(err error) string {
	var failure *afdProbeError
	if errors.As(err, &failure) {
		return failure.operation
	}
	return "setup"
}

func networkPolicyDenied(err error) bool {
	var status windows.NTStatus
	return errors.As(err, &status) && (uint32(status) == 0xc0000022 || uint32(status) == 0xc00000ca) // ACCESS_DENIED / NETWORK_ACCESS_DENIED
}
func afdOpenEA() [52]byte {
	var packet [52]byte
	packet[5] = 15 // EaNameLength excludes NUL.
	binary.LittleEndian.PutUint16(packet[6:8], 28)
	copy(packet[8:24], "AfdOpenPacketXX\x00")
	binary.LittleEndian.PutUint32(packet[24:28], afdIgnoreTDI)
	binary.LittleEndian.PutUint32(packet[32:36], 2) // AF_INET
	binary.LittleEndian.PutUint32(packet[36:40], 1) // SOCK_STREAM
	binary.LittleEndian.PutUint32(packet[40:44], 6) // IPPROTO_TCP
	return packet
}

// Native calls use a synchronous handle. The worker's parent-owned deadline and
// kill-on-close Job bound a pending connect; no input is read before this gate.
// Ordinary-process controls run in a separate deadline-bounded trusted child.
func nativeTCPProbe(address string) (bool, error) {
	target, err := netip.ParseAddrPort(address)
	if err != nil || !target.Addr().Is4() || target.Port() == 0 {
		return false, sandboxFailure("worker AFD probe address")
	}
	if unsafe.Sizeof(uintptr(0)) != 8 || unsafe.Sizeof(afdBindInfo{}) != 20 || unsafe.Sizeof(afdConnectInfo{}) != 40 || unsafe.Offsetof(afdConnectInfo{}.Address) != 24 {
		return false, sandboxFailure("worker AFD probe ABI")
	}
	name, err := windows.NewNTUnicodeString(`\Device\Afd\Endpoint`)
	if err != nil {
		return false, sandboxFailure("worker AFD device name")
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	packet := afdOpenEA()
	var pinned runtime.Pinner
	pinned.Pin(&packet[0])
	defer pinned.Unpin()
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.SYNCHRONIZE, &attributes, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN, windows.FILE_SYNCHRONOUS_IO_NONALERT, uintptr(unsafe.Pointer(&packet[0])), uint32(len(packet)))
	runtime.KeepAlive(packet)
	if err != nil {
		return false, &afdProbeError{"open", err}
	}
	defer windows.CloseHandle(handle)
	bind := afdBindInfo{Address: afdSockaddr{Family: 2}}
	var bound afdSockaddr
	if err = afdControl(handle, afdBind, unsafe.Pointer(&bind), uint32(unsafe.Sizeof(bind)), unsafe.Pointer(&bound), uint32(unsafe.Sizeof(bound))); err != nil {
		return false, &afdProbeError{"bind", err}
	}
	connect := afdConnectInfo{Address: afdSockaddr{Family: 2, Address: target.Addr().As4()}}
	binary.BigEndian.PutUint16(connect.Address.Port[:], target.Port())
	if err = afdControl(handle, afdConnect, unsafe.Pointer(&connect), uint32(unsafe.Sizeof(connect)), nil, 0); err != nil {
		return false, &afdProbeError{"connect", err}
	}
	return true, nil
}
func afdControl(handle windows.Handle, code uint32, input unsafe.Pointer, inputSize uint32, output unsafe.Pointer, outputSize uint32) error {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtDeviceIoControlFile")
	if proc.Find() != nil {
		return sandboxFailure("worker AFD API")
	}
	var io windows.IO_STATUS_BLOCK
	var pinned runtime.Pinner
	pinned.Pin(&io)
	pinned.Pin(input)
	if output != nil {
		pinned.Pin(output)
	}
	defer pinned.Unpin()
	result, _, _ := proc.Call(uintptr(handle), 0, 0, 0, uintptr(unsafe.Pointer(&io)), uintptr(code), uintptr(input), uintptr(inputSize), uintptr(output), uintptr(outputSize))
	status := windows.NTStatus(uint32(result))
	if status == windows.STATUS_PENDING {
		result, err := windows.WaitForSingleObject(handle, windows.INFINITE)
		if err != nil || result != windows.WAIT_OBJECT_0 {
			// A pending kernel request still owns the buffers: never return them to Go.
			// Terminate the worker/control child if waiting fails; exit tears down
			// pending I/O without reusing the kernel-owned Go buffers.
			os.Exit(104)
		}
		status = io.Status
	}
	runtime.KeepAlive(input)
	runtime.KeepAlive(output)
	if uint32(status)&0x80000000 != 0 {
		return status
	}
	return nil
}
