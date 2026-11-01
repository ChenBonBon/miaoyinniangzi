package sourceruntime

import (
	"encoding/json"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"miaoyinniangzi/internal/sourcenetwork"
)

const ResolveWorkerFlag = "--miaoyin-source-resolver"
const errResolve = "音源未能返回有效地址"
const errResolveRuntime = "音源解析运行失败或不兼容"
const MessageNetworkPolicy = "音源请求被安全策略拒绝，仅支持 HTTPS 公网地址"
const MessageServiceRestricted = "音源服务拒绝请求或请求过于频繁"

type ResolveInput struct {
	SongMID  string `json:"songmid"`
	SongID   string `json:"songId"`
	MediaMID string `json:"strMediaMid"`
	Name     string `json:"name"`
	Singer   string `json:"singer"`
	Album    string `json:"albumName"`
}
type Resolution struct {
	URL        string `json:"url"`
	Quality    string `json:"quality"`
	Error      string `json:"error"`
	RetryLower bool   `json:"retryLower"`
}
type resolveJob struct {
	Script  []byte       `json:"script"`
	Info    ResolveInput `json:"info"`
	Quality string       `json:"quality"`
}

// All interpreter activity stays on this goroutine. HTTP is a synchronous,
// bounded parent RPC; no Go network/file/crypto/OS objects are exposed to JS.
func resolveScript(job resolveJob, request func(sourcenetwork.Request) (sourcenetwork.Response, error)) (result Resolution) {
	result.Error = errResolveRuntime
	defer func() {
		if recover() != nil {
			result = Resolution{Error: errResolveRuntime}
		}
	}()
	vm := goja.New()
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(128)
	timer := time.AfterFunc(10*time.Second, func() { vm.Interrupt("limit") })
	defer timer.Stop()
	noop := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	console := vm.NewObject()
	for _, name := range []string{"log", "warn", "error", "info", "debug", "group", "groupEnd", "groupCollapsed", "trace", "time", "timeEnd", "clear"} {
		_ = console.Set(name, noop)
	}
	_ = vm.Set("console", console)
	lx := vm.NewObject()
	events := vm.NewObject()
	for _, name := range []string{"request", "inited", "updateAlert"} {
		_ = events.Set(name, name)
	}
	_ = lx.Set("EVENT_NAMES", events)
	_ = lx.Set("env", "desktop")
	_ = lx.Set("version", "2.9.0")
	_ = lx.Set("utils", vm.NewObject())
	var handler goja.Callable
	ready := false
	networkEnabled := false
	networkFailed := false
	_ = lx.Set("on", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).String() == "request" {
			handler, _ = goja.AssertFunction(call.Argument(1))
		}
		return goja.Undefined()
	})
	_ = lx.Set("send", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).String() != "inited" {
			return goja.Undefined()
		}
		data := call.Argument(1).ToObject(vm)
		if !data.Get("status").StrictEquals(vm.ToValue(true)) {
			return goja.Undefined()
		}
		providers := data.Get("sources").ToObject(vm)
		tx := providers.Get("tx").ToObject(vm)
		action := false
		quality := false
		for _, v := range stringsFromArray(tx.Get("actions")) {
			if v == "musicUrl" {
				action = true
			}
		}
		for _, v := range stringsFromArray(tx.Get("qualitys")) {
			if v == job.Quality {
				quality = true
			}
		}
		ready = action && quality
		return goja.Undefined()
	})
	_ = lx.Set("request", func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(2))
		if !ok {
			panic(vm.NewTypeError("unsupported callback"))
		}
		fail := func() { _, _ = callback(goja.Undefined(), vm.ToValue("音源网络请求失败"), goja.Undefined()) }
		// Initialization/update requests never get a network capability.
		if !networkEnabled {
			fail()
			return vm.ToValue(noop)
		}
		rawURL, ok := call.Argument(0).(goja.String)
		if !ok {
			networkFailed = true
			fail()
			return vm.ToValue(noop)
		}
		input := sourcenetwork.Request{URL: rawURL.String(), Method: "GET", Headers: map[string]string{}}
		options := call.Argument(1).ToObject(vm)
		if v := options.Get("method"); v != nil && !goja.IsUndefined(v) {
			input.Method = v.String()
		}
		if h, ok := options.Get("headers").(*goja.Object); ok {
			keys := h.Keys()
			if len(keys) > 32 {
				networkFailed = true
				fail()
				return vm.ToValue(noop)
			}
			for _, key := range keys {
				value, ok := h.Get(key).(goja.String)
				if !ok {
					networkFailed = true
					fail()
					return vm.ToValue(noop)
				}
				input.Headers[key] = value.String()
			}
		}
		if body := options.Get("body"); body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
			if text, ok := body.(goja.String); ok {
				input.Body = []byte(text.String())
			} else {
				var err error
				input.Body, err = json.Marshal(body.Export())
				if err != nil {
					networkFailed = true
					fail()
					return vm.ToValue(noop)
				}
				if input.Headers["Content-Type"] == "" && input.Headers["content-type"] == "" {
					input.Headers["Content-Type"] = "application/json"
				}
			}
		}
		response, err := request(input)
		if err != nil {
			networkFailed = true
			fail()
			return vm.ToValue(noop)
		}
		var body any = string(response.Body)
		var parsed any
		if json.Unmarshal(response.Body, &parsed) == nil {
			body = parsed
		}
		_, err = callback(goja.Undefined(), goja.Null(), vm.ToValue(map[string]any{"statusCode": response.Status, "body": body, "headers": map[string]string{}}))
		if err != nil {
			panic(vm.NewTypeError("callback failed"))
		}
		return vm.ToValue(noop)
	})
	_ = vm.Set("lx", lx)
	tasks := []*scheduledTask{}
	_ = vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		delay := call.Argument(1).ToInteger()
		if !ok || len(tasks) >= 32 || delay > 1000 {
			panic(vm.NewTypeError("timer limit"))
		}
		if delay < 0 {
			delay = 0
		}
		args := []goja.Value{}
		if len(call.Arguments) > 2 {
			args = append(args, call.Arguments[2:]...)
		}
		task := &scheduledTask{id: len(tasks) + 1, callback: fn, args: args, due: time.Now().Add(time.Duration(delay) * time.Millisecond)}
		tasks = append(tasks, task)
		return vm.ToValue(task.id)
	})
	_ = vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		index := call.Argument(0).ToInteger() - 1
		if index >= 0 && index < int64(len(tasks)) {
			tasks[index].canceled = true
		}
		return goja.Undefined()
	})
	drain := func() error {
		for len(tasks) > 0 {
			var next *scheduledTask
			for _, task := range tasks {
				if !task.canceled && (next == nil || task.due.Before(next.due)) {
					next = task
				}
			}
			if next == nil {
				return nil
			}
			if delay := time.Until(next.due); delay > 0 {
				time.Sleep(delay)
			}
			next.canceled = true
			if _, err := next.callback(goja.Undefined(), next.args...); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := vm.RunString(string(job.Script)); err != nil {
		return
	}
	if !ready {
		if err := drain(); err != nil {
			return
		}
	}
	if !ready || handler == nil {
		return
	}
	// Do not run startup/update timers after enabling networking.
	for _, task := range tasks {
		task.canceled = true
	}
	networkEnabled = true
	value, err := handler(goja.Undefined(), vm.ToValue(map[string]any{"action": "musicUrl", "source": "tx", "info": map[string]any{"type": job.Quality, "musicInfo": map[string]any{"songmid": job.Info.SongMID, "songId": job.Info.SongID, "strMediaMid": job.Info.MediaMID, "name": job.Info.Name, "singer": job.Info.Singer, "albumName": job.Info.Album, "source": "tx"}}}))
	if err != nil {
		return
	}
	if promise, ok := value.Export().(*goja.Promise); ok {
		if promise.State() == goja.PromiseStatePending {
			if err := drain(); err != nil {
				return
			}
		}
		if promise.State() != goja.PromiseStateFulfilled {
			result = Resolution{Error: errResolve, RetryLower: !networkFailed}
			return
		}
		value = promise.Result()
	}
	if networkFailed {
		return Resolution{Error: "音源网络请求失败"}
	}
	text, ok := value.(goja.String)
	if !ok {
		return Resolution{Error: errResolve, RetryLower: true}
	}
	if len(text.String()) == 0 || len(text.String()) > 8192 {
		return Resolution{Error: errResolve}
	}
	return Resolution{URL: text.String(), Quality: job.Quality}
}

func validQuality(q string) bool { return q == "flac" || q == "320k" || q == "128k" }
func validResolveInput(info ResolveInput) bool {
	if len(info.SongMID) == 0 || len(info.SongMID) > 64 {
		return false
	}
	for _, c := range info.SongMID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return len(info.SongID) <= 32 && len(info.MediaMID) <= 64 && len(info.Name) <= 400 && len(info.Singer) <= 1024 && len(info.Album) <= 1024
}

// Prevent accidental unsafe error propagation from the worker protocol.
func validResolution(r Resolution, q string) bool {
	if r.Error == "" {
		return r.Quality == q && len(r.URL) > 0 && len(r.URL) <= 8192 && !r.RetryLower
	}
	if r.URL != "" || r.Quality != "" {
		return false
	}
	for _, message := range []string{errResolve, errResolveRuntime, "音源网络请求失败", errSandbox, errProtocol} {
		if r.Error == message {
			return !r.RetryLower || message == errResolve
		}
	}
	return false
}
