// Package sourceruntime runs untrusted scripts in short-lived, restricted
// subprocesses. Scripts, keys and interpreter exceptions are never logged.
package sourceruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"miaoyinniangzi/internal/sourcenetwork"
	"os"
	"time"
)

const WorkerFlag = "--miaoyin-source-worker"
const maxScriptSize = 1024 * 1024
const maxResidentBytes = 192 * 1024 * 1024

type Report struct {
	Ready     bool     `json:"ready"`
	Qualities []string `json:"qualities"`
	Error     string   `json:"error"`
}
type Inspector interface {
	Inspect(context.Context, []byte) (Report, error)
}
type ProcessInspector struct {
	Executable string
	Network    sourcenetwork.Proxy
}
type workerRequest struct {
	Script []byte `json:"script"`
}

func NewInspector() (*ProcessInspector, error) {
	path, err := workerExecutable()
	if err != nil {
		return nil, errors.New("无法定位音源辅助进程")
	}
	return &ProcessInspector{Executable: path}, nil
}

// boundedBuffer prevents an unexpected/exploited worker from filling parent
// memory via stdout. Only the fixed, small report protocol is accepted.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 8192 {
		return 0, errors.New("worker output limit exceeded")
	}
	return b.Buffer.Write(data)
}
func (p *ProcessInspector) Inspect(ctx context.Context, script []byte) (Report, error) {
	if len(script) == 0 || len(script) > maxScriptSize {
		return Report{}, errors.New("音源脚本大小不符合限制")
	}
	payload, err := json.Marshal(workerRequest{Script: script})
	if err != nil {
		return Report{}, errors.New("无法准备音源校验")
	}
	defer clear(payload)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	worker, err := startWorker(ctx, p.Executable, WorkerFlag)
	if err != nil {
		if ctx.Err() != nil {
			return Report{}, errors.New("音源校验超时或已取消")
		}
		return Report{}, errors.New("无法启动音源隔离进程")
	}
	defer worker.close()
	var output boundedBuffer
	done := make(chan error, 1)
	go func() {
		_, writeErr := worker.input.Write(payload)
		worker.input.Close()
		_, readErr := io.Copy(&output, worker.output)
		if writeErr != nil || readErr != nil {
			worker.kill()
		}
		waitErr := worker.wait()
		if writeErr != nil {
			done <- writeErr
		} else if readErr != nil {
			done <- readErr
		} else {
			done <- waitErr
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				return Report{}, errors.New("音源校验超时或已取消")
			}
			if err != nil {
				return Report{}, errors.New("音源隔离进程异常退出")
			}
			var report Report
			decoder := json.NewDecoder(&output)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&report); err != nil {
				return Report{}, errors.New("音源校验返回无效结果")
			}
			if decoder.Decode(new(any)) != io.EOF {
				return Report{}, errors.New("音源校验返回无效结果")
			}
			// Reports may not carry exception text or arbitrary strings into the UI.
			if !validReport(report) {
				return Report{}, errors.New("音源校验返回无效结果")
			}
			return report, nil
		case <-ctx.Done():
			worker.kill()
			<-done
			return Report{}, errors.New("音源校验超时或已取消")
		case <-ticker.C:
			size, err := residentBytes(worker.pid)
			if err != nil {
				failures++
			} else {
				failures = 0
			}
			if size > maxResidentBytes || failures >= 5 {
				worker.kill()
				<-done
				return Report{}, errors.New("音源隔离进程超出资源限制或无法安全监控")
			}
		}
	}
}

const (
	errSandbox     = "音源进程安全限制初始化失败"
	errProtocol    = "音源校验输入无效"
	errScript      = "脚本无法在当前隔离运行器中初始化"
	errTimeout     = "脚本执行超时"
	errDeclaration = "脚本未完成有效的洛雪音源初始化"
	errQQ          = "脚本未声明可用的 QQ 音乐地址解析能力"
)

func validReport(r Report) bool {
	if r.Ready {
		if r.Error != "" || len(r.Qualities) == 0 || len(r.Qualities) > 3 {
			return false
		}
		seen := map[string]bool{}
		for _, q := range r.Qualities {
			if (q != "flac" && q != "320k" && q != "128k") || seen[q] {
				return false
			}
			seen[q] = true
		}
		return true
	}
	for _, message := range []string{errSandbox, errProtocol, errScript, errTimeout, errDeclaration, errQQ} {
		if r.Error == message && len(r.Qualities) == 0 {
			return true
		}
	}
	return false
}

// WorkerMain must be entered before Wails, SQLite or Keychain initialization.
// Input arrives through an anonymous pipe, never arguments or a temporary file.
func WorkerMain() int {
	report := Report{Qualities: []string{}}
	if sandboxWorker() != nil {
		report.Error = errSandbox
	} else {
		decoder := json.NewDecoder(io.LimitReader(os.Stdin, 2*maxScriptSize))
		decoder.DisallowUnknownFields()
		var request workerRequest
		if decoder.Decode(&request) != nil || len(request.Script) == 0 || len(request.Script) > maxScriptSize || decoder.Decode(new(any)) != io.EOF {
			report.Error = errProtocol
		} else {
			report = inspectScript(request.Script)
		}
		clear(request.Script)
	}
	if json.NewEncoder(os.Stdout).Encode(report) != nil {
		return 1
	}
	return 0
}
