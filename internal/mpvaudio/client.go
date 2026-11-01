package mpvaudio

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type nativeEvent struct {
	ID       int32
	Error    int32
	Userdata uint64
	Data     unsafe.Pointer
}
type nativeEnd struct {
	Reason int32
	Error  int32
}
type api struct {
	create       func() uintptr
	initialize   func(uintptr) int32
	option       func(uintptr, string, string) int32
	command      func(uintptr, unsafe.Pointer) int32
	commandAsync func(uintptr, uint64, unsafe.Pointer) int32
	get          func(uintptr, string) *byte
	wait         func(uintptr, float64) *nativeEvent
	release      func(unsafe.Pointer)
	destroy      func(uintptr)
}

var libraries = struct {
	sync.Mutex
	values map[string]*api
}{values: make(map[string]*api)}

type libraryClient struct {
	api    *api
	handle uintptr
}

func bindLibrary(handle uintptr) (value *api, err error) {
	// Missing symbols/ABI mismatch are sanitized; never print native diagnostics.
	defer func() {
		if recover() != nil {
			value = nil
			err = ErrLibrary
		}
	}()
	var version func() uint32
	purego.RegisterLibFunc(&version, handle, "mpv_client_api_version")
	if version()>>16 != 2 {
		return nil, ErrLibrary
	}
	a := &api{}
	purego.RegisterLibFunc(&a.create, handle, "mpv_create")
	purego.RegisterLibFunc(&a.initialize, handle, "mpv_initialize")
	purego.RegisterLibFunc(&a.option, handle, "mpv_set_option_string")
	purego.RegisterLibFunc(&a.command, handle, "mpv_command")
	purego.RegisterLibFunc(&a.commandAsync, handle, "mpv_command_async")
	purego.RegisterLibFunc(&a.get, handle, "mpv_get_property_string")
	purego.RegisterLibFunc(&a.wait, handle, "mpv_wait_event")
	purego.RegisterLibFunc(&a.release, handle, "mpv_free")
	purego.RegisterLibFunc(&a.destroy, handle, "mpv_terminate_destroy")
	return a, nil
}

// Warmup loads/binds code only: no player, device, media, or network is opened.
func Warmup() error {
	_, err := libraryAPI(LibraryPath())
	return err
}
func libraryAPI(path string) (*api, error) {
	if path == "" {
		return nil, ErrLibrary
	}
	libraries.Lock()
	a := libraries.values[path]
	if a == nil {
		handle, err := loadLibrary(path)
		if err != nil {
			libraries.Unlock()
			return nil, ErrLibrary
		}
		a, err = bindLibrary(handle)
		if err != nil {
			closeLibrary(handle)
			libraries.Unlock()
			return nil, ErrLibrary
		}
		// Keep loaded code for process lifetime, including asynchronous teardown.
		libraries.values[path] = a
	}
	libraries.Unlock()
	return a, nil
}
func newClient(path string) (client, error) {
	a, err := libraryAPI(path)
	if err != nil {
		return nil, err
	}
	handle := a.create()
	if handle == 0 {
		return nil, ErrPlayer
	}
	return &libraryClient{api: a, handle: handle}, nil
}
func (c *libraryClient) option(k, v string) bool { return c.api.option(c.handle, k, v) >= 0 }
func (c *libraryClient) initialize() bool        { return c.api.initialize(c.handle) >= 0 }
func (c *libraryClient) arguments(args []string, async bool) bool {
	buffers := make([][]byte, len(args))
	pointers := make([]*byte, len(args)+1)
	for i, arg := range args {
		buffers[i] = append([]byte(arg), 0)
		pointers[i] = &buffers[i][0]
	}
	var result int32
	if async {
		result = c.api.commandAsync(c.handle, 0, unsafe.Pointer(&pointers[0]))
	} else {
		result = c.api.command(c.handle, unsafe.Pointer(&pointers[0]))
	}
	runtime.KeepAlive(buffers)
	runtime.KeepAlive(pointers)
	return result >= 0
}
func (c *libraryClient) command(args ...string) bool { return c.arguments(args, false) }
func (c *libraryClient) get(key string) string {
	value := c.api.get(c.handle, key)
	if value == nil {
		return ""
	}
	defer c.api.release(unsafe.Pointer(value))
	// All requested properties are tiny scalar values, not titles/URLs/scripts.
	bytes := unsafe.Slice(value, 16*1024)
	for i, b := range bytes {
		if b == 0 {
			return string(bytes[:i])
		}
	}
	return ""
}
func (c *libraryClient) poll() event {
	value := c.api.wait(c.handle, 0)
	if value == nil {
		return event{}
	}
	result := event{id: int(value.ID)}
	if value.ID == 7 && value.Data != nil {
		end := (*nativeEnd)(value.Data)
		if end.Reason == 4 {
			result.failure = int(end.Error)
			if result.failure >= 0 {
				result.failure = -1
			}
		}
	}
	return result
}
func (c *libraryClient) destroy() {
	// Stop is asynchronous so cancelling a stalled network read cannot block the
	// app mutex. libmpv owns its threads until terminate_destroy has joined them.
	c.arguments([]string{"stop"}, true)
	go c.api.destroy(c.handle)
}
