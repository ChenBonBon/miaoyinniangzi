//go:build !darwin

package nativeaudio

import "errors"

var ErrPlayer = errors.New("当前平台不支持原生 FLAC 播放")

type State struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Playing  bool    `json:"playing"`
	Ended    bool    `json:"ended"`
}
type Player struct{}

func Open(string) (*Player, error)                     { return nil, ErrPlayer }
func (*Player) Control(string, float64) (State, error) { return State{}, ErrPlayer }
func (*Player) Close()                                 {}
