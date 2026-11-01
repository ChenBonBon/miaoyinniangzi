package main

import (
	"context"
	"testing"
	"time"
)

func TestRealQuitWaitsForMatchingRendererAcknowledgement(t *testing.T) {
	app := &App{}
	emitted := make(chan string, 1)
	done := make(chan bool, 1)
	go func() {
		done <- app.synchronizeQuit(context.Background(), func(id string) { emitted <- id }, time.Second)
	}()
	id := <-emitted
	if app.AcknowledgeQuit("wrong") {
		t.Fatal("wrong acknowledgement accepted")
	}
	select {
	case <-done:
		t.Fatal("quit did not wait for latest playback save")
	default:
	}
	if !app.synchronizeQuit(context.Background(), func(string) { t.Error("duplicate quit emitted") }, time.Second) {
		t.Fatal("overlapping quit not guarded")
	}
	if !app.AcknowledgeQuit(id) || app.AcknowledgeQuit(id) {
		t.Fatal("acknowledgement not exactly once")
	}
	select {
	case prevent := <-done:
		if prevent {
			t.Fatal("real quit was converted into hide")
		}
	case <-time.After(time.Second):
		t.Fatal("quit acknowledgement blocked")
	}
}
func TestQuitCannotHangOnUnresponsiveRenderer(t *testing.T) {
	app := &App{}
	started := time.Now()
	var id string
	if app.synchronizeQuit(context.Background(), func(value string) { id = value }, 20*time.Millisecond) {
		t.Fatal("quit blocked")
	}
	if time.Since(started) > time.Second {
		t.Fatal("quit timeout unbounded")
	}
	if app.AcknowledgeQuit(id) {
		t.Fatal("expired acknowledgement accepted")
	}
}
func TestQuitHonorsCanceledApplicationContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if (&App{}).synchronizeQuit(ctx, func(string) {}, time.Second) {
		t.Fatal("canceled app prevented exit")
	}
}
