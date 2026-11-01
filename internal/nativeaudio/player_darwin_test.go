//go:build darwin

package nativeaudio

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func signalFixture(t *testing.T) string {
	t.Helper()
	const rate = 44100
	data := make([]byte, 44+rate*6*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	for i := 0; i < rate*6; i++ {
		frequency := 220.
		if i >= rate*2 {
			frequency = 880
		}
		if i >= rate*4 {
			frequency = 1760
		}
		sample := int16(10000 * math.Sin(2*math.Pi*frequency*float64(i)/rate))
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(sample))
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "signal.wav")
	flac := filepath.Join(dir, "signal.flac")
	if err := os.WriteFile(wav, data, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("afconvert", "-f", "flac", "-d", "flac", wav, flac).CombinedOutput(); err != nil {
		t.Fatalf("fixture encoder failed: %s", output)
	}
	return flac
}
func TestFLACSampleSeekMatchesDecodedSound(t *testing.T) {
	file := signalFixture(t)
	for _, test := range []struct{ seconds, frequency float64 }{{.5, 220}, {3, 880}, {5.7, 1760}, {.7, 220}, {5.8, 1760}} {
		frequency := decodedFrequency(file, test.seconds)
		if math.Abs(frequency-test.frequency) > 20 {
			t.Fatalf("seek %.2fs decoded %.0fHz, expected %.0fHz", test.seconds, frequency, test.frequency)
		}
	}
}
func TestNativePlayerNearEndPausedAndPlaying(t *testing.T) {
	if os.Getenv("MIAOYIN_NATIVE_AUDIO_SMOKE") != "1" {
		t.Skip("native output device opt-in")
	}
	player, err := Open(signalFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	state, err := player.Control("seek", 5.7)
	if err != nil || math.Abs(state.Position-5.7) > .001 || state.Playing {
		t.Fatalf("paused seek mismatch %+v %v", state, err)
	}
	player.Control("volume", 0)
	if _, err = player.Control("play", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	state, err = player.Control("pause", 0)
	if err != nil || state.Position < 5.7 || state.Position > 6 || state.Playing {
		t.Fatalf("native progress mismatch %+v %v", state, err)
	}
	player.Control("seek", .5)
	player.Control("play", 0)
	time.Sleep(80 * time.Millisecond)
	state, err = player.Control("seek", 5.6)
	if err != nil || math.Abs(state.Position-5.6) > .03 || !state.Playing {
		t.Fatalf("playing seek mismatch %+v %v", state, err)
	}
	player.Control("pause", 0)
	state, err = player.Control("seek", 6)
	if err != nil || state.Position > 6 || state.Ended || state.Playing {
		t.Fatalf("end seek changed playback unexpectedly %+v %v", state, err)
	}
}
