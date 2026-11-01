//go:build darwin

// Native opt-in smoke tool. It never opens the player's database or source keys.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"miaoyinniangzi/internal/windowlife"
)

type Probe struct {
	ack  chan struct{}
	once sync.Once
}

func (p *Probe) Ack() { p.once.Do(func() { close(p.ack) }) }
func main() {
	var current atomic.Pointer[context.Context]
	var ready, quits, ticks atomic.Int64
	probe := &Probe{ack: make(chan struct{})}
	results := map[string]bool{}
	var resultsMu sync.Mutex
	setResult := func(name string) { resultsMu.Lock(); results[name] = true; resultsMu.Unlock() }
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(35 * time.Second):
			fmt.Println("native lifecycle smoke timed out")
			os.Exit(2)
		}
	}()
	wait := func(name string, predicate func() bool) {
		deadline := time.Now().Add(8 * time.Second)
		for !predicate() {
			if time.Now().After(deadline) {
				fmt.Println(name, "failed")
				os.Exit(2)
			}
			time.Sleep(25 * time.Millisecond)
		}
		setResult(name)
	}
	assets := fstest.MapFS{"index.html": {Data: []byte(`<!doctype html><html><body>Native lifecycle smoke<script>window.runtime.EventsOn('smoke:quit',()=>window.go.main.Probe.Ack());</script></body></html>`)}}
	app := &options.App{Title: "Window Lifecycle Smoke", Width: 720, Height: 600, AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{probe}, OnStartup: func(ctx context.Context) {
		current.Store(&ctx)
		go func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-ticker.C:
					ticks.Add(1)
				}
			}
		}()
	}, OnDomReady: func(context.Context) {
		if ready.Add(1) != 1 {
			return
		}
		go func() {
			wait("initial_window", func() bool { return nativeFlags()&2 != 0 })
			before := ticks.Load()
			nativeAction(1)
			wait("native_close_hides", func() bool { return nativeFlags()&1 != 0 })
			time.Sleep(250 * time.Millisecond)
			wait("background_context_alive", func() bool { return ticks.Load() > before+5 && quits.Load() == 0 })
			nativeAction(2)
			wait("reopen_after_close", func() bool { flags := nativeFlags(); return flags&1 == 0 && flags&2 != 0 })
			nativeAction(3)
			wait("minimized", func() bool { return nativeFlags()&4 != 0 })
			nativeAction(2)
			wait("reopen_after_minimize", func() bool { flags := nativeFlags(); return flags&1 == 0 && flags&2 != 0 && flags&4 == 0 })
			nativeAction(4)
			wait("cmd_w_hides", func() bool { return nativeFlags()&1 != 0 })
			wait("webview_not_reloaded", func() bool { return ready.Load() == 1 })
			action := 5
			if len(os.Args) > 1 && os.Args[1] == "menu" {
				action = 6
			}
			nativeAction(action)
		}()
	}}
	windowlife.Configure(app, func() context.Context {
		value := current.Load()
		if value == nil {
			return nil
		}
		return *value
	}, func(ctx context.Context) bool {
		quits.Add(1)
		runtime.EventsEmit(ctx, "smoke:quit")
		select {
		case <-probe.ack:
			setResult("hidden_renderer_acknowledges_quit")
		case <-time.After(2 * time.Second):
		}
		return false
	})
	if err := wails.Run(app); err != nil {
		fmt.Println("native lifecycle startup failed")
		os.Exit(2)
	}
	resultsMu.Lock()
	defer resultsMu.Unlock()
	if quits.Load() != 1 || !results["hidden_renderer_acknowledges_quit"] {
		fmt.Println("native quit failed")
		os.Exit(2)
	}
	results["real_quit_exits"] = true
	_ = json.NewEncoder(os.Stdout).Encode(results)
}
