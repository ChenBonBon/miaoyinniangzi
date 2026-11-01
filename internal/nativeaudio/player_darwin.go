//go:build darwin

// Package nativeaudio provides sample-frame-accurate FLAC playback. No URL,
// network authority, script, or untrusted error text reaches this boundary.
package nativeaudio

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AVFoundation -framework Foundation -framework AudioToolbox
#include <stdlib.h>
#include "player.h"
*/
import "C"
import (
	"errors"
	"math"
	"sync"
	"unsafe"
)

var ErrPlayer = errors.New("原生 FLAC 播放器操作失败，请重试")

type State struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Playing  bool    `json:"playing"`
	Ended    bool    `json:"ended"`
}
type Player struct {
	mu     sync.Mutex
	handle unsafe.Pointer
}

func Open(path string) (*Player, error) {
	value := C.CString(path)
	defer C.free(unsafe.Pointer(value))
	handle := C.miaoyin_audio_open(value)
	if handle == nil {
		return nil, ErrPlayer
	}
	return &Player{handle: handle}, nil
}
func (p *Player) Control(action string, value float64) (State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == nil {
		return State{}, ErrPlayer
	}
	switch action {
	case "play":
		if C.miaoyin_audio_play(p.handle) == 0 {
			return State{}, ErrPlayer
		}
	case "pause":
		C.miaoyin_audio_pause(p.handle)
	case "seek":
		if math.IsNaN(value) || math.IsInf(value, 0) || C.miaoyin_audio_seek(p.handle, C.double(value)) == 0 {
			return State{}, ErrPlayer
		}
	case "volume":
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return State{}, ErrPlayer
		}
		C.miaoyin_audio_volume(p.handle, C.double(value))
	case "state":
	default:
		return State{}, ErrPlayer
	}
	s := C.miaoyin_audio_state(p.handle)
	return State{Position: float64(s.position), Duration: float64(s.duration), Playing: s.playing != 0, Ended: s.ended != 0}, nil
}
func decodedFrequency(path string, seconds float64) float64 {
	value := C.CString(path)
	defer C.free(unsafe.Pointer(value))
	return float64(C.miaoyin_audio_probe_frequency(value, C.double(seconds)))
}
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle != nil {
		C.miaoyin_audio_close(p.handle)
		p.handle = nil
	}
}
