package sourceruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--miaoyin-sandbox-broker-test" {
		os.Exit(windowsBrokerMain())
	}
	if len(os.Args) > 2 && os.Args[1] == WorkerFlag && os.Args[2] == "--native-windows-probe" {
		os.Exit(windowsProbeMain())
	}
	if len(os.Args) > 1 && os.Args[1] == WorkerFlag {
		if len(os.Args) > 2 && os.Args[2] == "--sandbox-probe" {
			if sandboxWorker() != nil {
				os.Exit(2)
			}
			_, fileErr := os.ReadFile("/etc/hosts")
			connection, networkErr := net.DialTimeout("tcp", "127.0.0.1:9", 100*time.Millisecond)
			if connection != nil {
				connection.Close()
			}
			process, execErr := os.StartProcess("/usr/bin/true", []string{"true"}, &os.ProcAttr{Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}})
			if process != nil {
				process.Kill()
				process.Wait()
			}
			_ = json.NewEncoder(os.Stdout).Encode([]bool{os.IsPermission(fileErr), errors.Is(networkErr, syscall.EPERM) || errors.Is(networkErr, syscall.EACCES), execErr != nil})
			os.Exit(0)
		}
		os.Exit(WorkerMain())
	}
	if len(os.Args) > 1 && os.Args[1] == ResolveWorkerFlag {
		os.Exit(ResolveWorkerMain())
	}
	os.Exit(m.Run())
}

const validScript = `
 const {EVENT_NAMES,on,send}=lx;
 on(EVENT_NAMES.request,({action,source,info})=>Promise.resolve('https://example.invalid/audio'));
 send(EVENT_NAMES.inited,{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['atmos','128k','flac','320k']}}});`

func TestInspectSupportedQQDeclaration(t *testing.T) {
	for _, script := range []string{validScript, `Promise.resolve().then(()=>{` + validScript + `})`, `setTimeout(()=>{` + validScript + `},1)`} {
		report := inspectScript([]byte(script))
		if !report.Ready || strings.Join(report.Qualities, ",") != "flac,320k,128k" {
			t.Fatalf("inspection: %+v", report)
		}
	}
}
func TestRejectMissingCapabilitiesAndRedactErrors(t *testing.T) {
	for _, script := range []string{`throw new Error('SECRET-DO-NOT-LEAK')`, `lx.send('inited',{status:false})`, `lx.on('request',()=>{});lx.send('inited',{status:true,sources:{wy:{actions:['musicUrl'],qualitys:['flac']}}})`, `lx.send('inited',{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac']}}})`, `require('fs').readFileSync('/etc/passwd')`, `fetch('https://example.invalid')`} {
		report := inspectScript([]byte(script))
		if report.Ready || !validReport(report) || strings.Contains(report.Error, "SECRET") {
			t.Fatalf("invalid failure: %+v", report)
		}
	}
}
func TestInspectionDoesNotFetchScriptURLs(t *testing.T) {
	report := inspectScript([]byte(`lx.request('http://127.0.0.1:1',{headers:{'x-key':'SECRET'}},(err)=>{if(!err)throw Error('network allowed')});` + validScript))
	if !report.Ready {
		t.Fatalf("inspection failed: %+v", report)
	}
}
func TestInfiniteLoopIsInterrupted(t *testing.T) {
	start := time.Now()
	report := inspectScript([]byte(`for(;;){}`))
	if report.Ready || report.Error != errTimeout || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout: %+v", report)
	}
}
func TestIsolatedWorkerRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("implemented OS sandbox required")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	inspector := &ProcessInspector{Executable: executable}
	report, err := inspector.Inspect(context.Background(), []byte(validScript))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready {
		t.Fatalf("worker report: %+v", report)
	}
}
func TestWorkerCancellation(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("implemented OS sandbox required")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	inspector := &ProcessInspector{Executable: executable}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := inspector.Inspect(ctx, []byte(`for(;;){}`)); err == nil {
		t.Fatal("ignored cancellation")
	}
}
