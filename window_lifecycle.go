package main

import (
	"context"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) windowContext() context.Context { a.mu.RLock(); defer a.mu.RUnlock(); return a.ctx }
func (a *App) beforeQuit(ctx context.Context) bool {
	return a.synchronizeQuit(ctx, func(id string) { runtime.EventsEmit(ctx, "app:before-quit", id) }, 2*time.Second)
}

// Closing the macOS window never reaches this path. A real quit gives the
// renderer a bounded opportunity to stop audio and flush its latest position.
func (a *App) synchronizeQuit(ctx context.Context, emit func(string), timeout time.Duration) bool {
	a.mu.Lock()
	if a.quitting {
		a.mu.Unlock()
		return true
	}
	a.quitting = true
	a.quitSequence++
	id := strconv.FormatUint(a.quitSequence, 10)
	done := make(chan struct{})
	a.quitID = id
	a.quitDone = done
	a.mu.Unlock()
	emit(id)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-ctx.Done():
	case <-timer.C:
	}
	a.mu.Lock()
	a.quitDone = nil
	a.quitID = ""
	a.mu.Unlock()
	return false
}
func (a *App) AcknowledgeQuit(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || id != a.quitID || a.quitDone == nil {
		return false
	}
	close(a.quitDone)
	a.quitDone = nil
	return true
}
