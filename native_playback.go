package main

import (
	"errors"
	"miaoyinniangzi/internal/mpvaudio"
	"time"
)

func (a *App) clearNativeLocked() {
	a.playTrace = nil
	if a.nativePlayer != nil {
		a.nativePlayer.Close()
		a.nativePlayer = nil
	}
}
func (a *App) NativePlayback(requestID, action string, value float64) (mpvaudio.State, error) {
	a.mu.RLock()
	player := a.nativePlayer
	trace := a.playTrace
	valid := requestID != "" && a.playID == requestID && player != nil
	a.mu.RUnlock()
	if !valid {
		return mpvaudio.State{}, errors.New("播放会话已失效")
	}
	// A network seek must not retain the application mutex: cancellation and
	// source revocation can detach and close this player while it waits.
	started := time.Now()
	state, err := player.Control(action, value)
	if action == "play" {
		trace.FirstPlay(started, err, state.Playing, state.Buffering)
	}
	return state, err
}
