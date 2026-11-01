package sourceruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"miaoyinniangzi/internal/sourcenetwork"
)

type Resolver interface {
	Resolve(context.Context, []byte, ResolveInput, string) (Resolution, error)
}
type resolveFrame struct {
	Kind     string                  `json:"kind"`
	Request  *sourcenetwork.Request  `json:"request,omitempty"`
	Response *sourcenetwork.Response `json:"response,omitempty"`
	Result   *Resolution             `json:"result,omitempty"`
	Failed   bool                    `json:"failed,omitempty"`
}

func decodeLine(reader *bufio.Reader, target any) error {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid frame")
	}
	return nil
}
func (p *ProcessInspector) Resolve(ctx context.Context, script []byte, info ResolveInput, quality string) (Resolution, error) {
	if len(script) == 0 || len(script) > maxScriptSize || !validQuality(quality) || !validResolveInput(info) {
		return Resolution{}, errors.New("音源解析参数无效")
	}
	proxy := p.Network
	if proxy == nil {
		proxy = sourcenetwork.New()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	payload, err := json.Marshal(resolveJob{Script: script, Info: info, Quality: quality})
	if err != nil {
		return Resolution{}, errors.New("无法准备音源解析")
	}
	defer clear(payload)
	worker, err := startWorker(ctx, p.Executable, ResolveWorkerFlag)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Resolution{}, ErrResolveTimeout
		}
		if ctx.Err() != nil {
			return Resolution{}, ErrResolveCanceled
		}
		return Resolution{}, errors.New("无法启动音源隔离进程")
	}
	defer worker.close()
	input, output := worker.input, worker.output
	type outcome struct {
		result Resolution
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, exchangeErr := exchangeResolve(ctx, input, output, payload, proxy, quality)
		input.Close()
		output.Close()
		if exchangeErr != nil {
			worker.kill()
		}
		waitErr := worker.wait()
		if exchangeErr != nil {
			if errors.Is(exchangeErr, io.EOF) && waitErr != nil {
				done <- outcome{err: ErrResolveProcess}
				return
			}
			done <- outcome{err: ErrResolveProtocol}
			return
		}
		if waitErr != nil {
			done <- outcome{err: ErrResolveProcess}
			return
		}
		done <- outcome{result: result}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case value := <-done:
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return Resolution{}, ErrResolveTimeout
				}
				return Resolution{}, ErrResolveCanceled
			}
			if value.err == nil && value.result.Error == "" {
				if err := proxy.ValidateMedia(ctx, value.result.URL); err != nil {
					if errors.Is(err, sourcenetwork.ErrDenied) {
						return Resolution{}, ErrResolveMedia
					}
					return Resolution{}, ErrResolveMediaDNS
				}
			}
			return value.result, value.err
		case <-ctx.Done():
			worker.kill()
			<-done
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Resolution{}, ErrResolveTimeout
			}
			return Resolution{}, ErrResolveCanceled
		case <-ticker.C:
			size, err := residentBytes(worker.pid)
			if err != nil {
				failures++
			} else {
				failures = 0
			}
			if size > maxResidentBytes || failures >= 5 {
				worker.kill()
				cancel()
				<-done
				return Resolution{}, ErrResolveResources
			}
		}
	}
}
func exchangeResolve(ctx context.Context, input io.Writer, output io.Reader, payload []byte, proxy sourcenetwork.Proxy, quality string) (Resolution, error) {
	if _, err := input.Write(payload); err != nil {
		return Resolution{}, err
	}
	if _, err := input.Write([]byte{'\n'}); err != nil {
		return Resolution{}, err
	}
	reader := bufio.NewReaderSize(output, 192*1024)
	encoder := json.NewEncoder(input)
	var final *Resolution
	calls := 0
	networkFailure := ""
	for {
		var frame resolveFrame
		err := decodeLine(reader, &frame)
		if err != nil {
			return Resolution{}, err
		}
		if final != nil {
			return Resolution{}, errors.New("extra frame")
		}
		switch frame.Kind {
		case "http":
			calls++
			if frame.Request == nil || frame.Response != nil || frame.Result != nil || frame.Failed || calls > 6 || networkFailure != "" {
				return Resolution{}, errors.New("invalid request frame")
			}
			response, err := proxy.Do(ctx, *frame.Request)
			// Never send network errors (which could contain keys/URLs) to the child.
			reply := resolveFrame{Kind: "response", Failed: err != nil}
			if err == nil {
				reply.Response = &response
			} else {
				networkFailure = "音源网络请求失败"
				if errors.Is(err, sourcenetwork.ErrDenied) {
					networkFailure = MessageNetworkPolicy
				}
				if errors.Is(err, sourcenetwork.ErrRestricted) {
					networkFailure = MessageServiceRestricted
				}
			}
			if encoder.Encode(reply) != nil {
				return Resolution{}, errors.New("reply failed")
			}
		case "result":
			if frame.Result == nil || frame.Request != nil || frame.Response != nil || frame.Failed || !validResolution(*frame.Result, quality) {
				return Resolution{}, errors.New("invalid result frame")
			}
			final = frame.Result
			if networkFailure != "" {
				*final = Resolution{Error: networkFailure}
			}
			// Reject trailing bytes, duplicate results and workers that do not exit.
			if _, err := reader.ReadByte(); err != io.EOF {
				return Resolution{}, errors.New("trailing output")
			}
			return *final, nil
		default:
			return Resolution{}, errors.New("invalid frame kind")
		}
	}
}
func ResolveWorkerMain() int {
	encoder := json.NewEncoder(os.Stdout)
	finish := func(result Resolution) int {
		if encoder.Encode(resolveFrame{Kind: "result", Result: &result}) != nil {
			return 1
		}
		return 0
	}
	if sandboxWorker() != nil {
		return finish(Resolution{Error: errSandbox})
	}
	reader := bufio.NewReaderSize(os.Stdin, 2*maxScriptSize)
	var job resolveJob
	if decodeLine(reader, &job) != nil || len(job.Script) == 0 || len(job.Script) > maxScriptSize || !validQuality(job.Quality) || !validResolveInput(job.Info) {
		return finish(Resolution{Error: errProtocol})
	}
	defer clear(job.Script)
	calls := 0
	result := resolveScript(job, func(request sourcenetwork.Request) (sourcenetwork.Response, error) {
		calls++
		if calls > 6 {
			return sourcenetwork.Response{}, errors.New("limit")
		}
		// Bound stdout before marshaling large script-controlled fields.
		if len(request.URL) > 8192 || len(request.Body) > 64*1024 || len(request.Headers) > 32 {
			return sourcenetwork.Response{}, errors.New("limit")
		}
		total := 0
		for key, value := range request.Headers {
			total += len(key) + len(value)
		}
		if total > 16*1024 {
			return sourcenetwork.Response{}, errors.New("limit")
		}
		if encoder.Encode(resolveFrame{Kind: "http", Request: &request}) != nil {
			return sourcenetwork.Response{}, errors.New("pipe")
		}
		var reply resolveFrame
		if decodeLine(reader, &reply) != nil || reply.Kind != "response" || reply.Request != nil || reply.Result != nil || reply.Failed || reply.Response == nil || len(reply.Response.Body) > sourcenetwork.MaxBody {
			return sourcenetwork.Response{}, errors.New("network")
		}
		return *reply.Response, nil
	})
	return finish(result)
}
