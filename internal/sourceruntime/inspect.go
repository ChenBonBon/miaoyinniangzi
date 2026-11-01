package sourceruntime

import (
	"errors"
	"strconv"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

type scheduledTask struct {
	id       int
	callback goja.Callable
	args     []goja.Value
	due      time.Time
	canceled bool
}

// inspectScript exposes a deliberately bounded subset of the lx API. Network
// calls always fail locally during inspection; no script URL is ever fetched.
func inspectScript(script []byte) (report Report) {
	report.Qualities = []string{}
	report.Error = errScript
	defer func() {
		if recover() != nil {
			report = Report{Qualities: []string{}, Error: errScript}
		}
	}()
	vm := goja.New()
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(128)
	deadline := time.Now().Add(time.Second)
	timer := time.AfterFunc(time.Second, func() { vm.Interrupt("execution limit") })
	defer timer.Stop()
	initialized := false
	validInit := false
	handler := false
	qq := false
	var qualities []string
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
	_ = lx.Set("on", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).StrictEquals(vm.ToValue("request")) {
			_, handler = goja.AssertFunction(call.Argument(1))
		}
		return goja.Undefined()
	})
	_ = lx.Set("send", func(call goja.FunctionCall) goja.Value {
		if !call.Argument(0).StrictEquals(vm.ToValue("inited")) {
			return goja.Undefined()
		}
		initialized = true
		validInit = false
		qq = false
		qualities = nil
		payload, ok := call.Argument(1).(*goja.Object)
		if !ok {
			return goja.Undefined()
		}
		if !payload.Get("status").StrictEquals(vm.ToValue(true)) {
			return goja.Undefined()
		}
		validInit = true
		providers, ok := payload.Get("sources").(*goja.Object)
		if !ok {
			return goja.Undefined()
		}
		tx, ok := providers.Get("tx").(*goja.Object)
		if !ok {
			return goja.Undefined()
		}
		actions := stringsFromArray(tx.Get("actions"))
		for _, action := range actions {
			if action == "musicUrl" {
				qq = true
			}
		}
		available := stringsFromArray(tx.Get("qualitys"))
		for _, quality := range []string{"flac", "320k", "128k"} {
			for _, candidate := range available {
				if candidate == quality {
					qualities = append(qualities, quality)
					break
				}
			}
		}
		return goja.Undefined()
	})
	_ = lx.Set("request", func(call goja.FunctionCall) goja.Value {
		// Do not inspect, log or return the requested URL or headers (may contain keys).
		if callback, ok := goja.AssertFunction(call.Argument(2)); ok {
			_, _ = callback(goja.Undefined(), vm.ToValue("兼容性校验期间禁止网络请求"), goja.Undefined())
		}
		return vm.ToValue(noop)
	})
	_ = vm.Set("lx", lx)
	tasks := []*scheduledTask{}
	_ = vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok || len(tasks) >= 32 {
			panic(vm.NewTypeError("unsupported timer"))
		}
		delay := call.Argument(1).ToInteger()
		if delay < 0 {
			delay = 0
		}
		if delay > 1000 {
			panic(vm.NewTypeError("initialization timer exceeds limit"))
		}
		args := []goja.Value{}
		if len(call.Arguments) > 2 {
			args = append(args, call.Arguments[2:]...)
		}
		task := &scheduledTask{id: len(tasks) + 1, callback: callback, args: args, due: time.Now().Add(time.Duration(delay) * time.Millisecond)}
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
	_, err := vm.RunString(string(script))
	for err == nil && !initialized {
		var next *scheduledTask
		for _, task := range tasks {
			if !task.canceled && (next == nil || task.due.Before(next.due)) {
				next = task
			}
		}
		if next == nil {
			break
		}
		if next.due.After(deadline) {
			report.Error = errTimeout
			return
		}
		if delay := time.Until(next.due); delay > 0 {
			time.Sleep(delay)
		}
		next.canceled = true
		_, err = next.callback(goja.Undefined(), next.args...)
	}
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			report.Error = errTimeout
		}
		return
	}
	if !initialized || !validInit || !handler {
		report.Error = errDeclaration
		return
	}
	if !qq || len(qualities) == 0 {
		report.Error = errQQ
		return
	}
	report.Ready = true
	report.Qualities = qualities
	report.Error = ""
	return
}

func stringsFromArray(value goja.Value) []string {
	object, ok := value.(*goja.Object)
	if !ok {
		return nil
	}
	size := object.Get("length").ToInteger()
	if size < 0 || size > 64 {
		return nil
	}
	result := []string{}
	for index := int64(0); index < size; index++ {
		// Accesses run on the VM thread with the interrupt timer still armed.
		item := object.Get(strconv.FormatInt(index, 10))
		if _, ok := item.(goja.String); ok {
			result = append(result, item.String())
		}
	}
	return result
}
