package downloads

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMP3QualityUsesFramesNotRequestedLabel(t *testing.T) {
	for _, sample := range []struct {
		header byte
		length int
		want   string
	}{{224, 1044, "320k"}, {144, 417, "128k"}} {
		frame := make([]byte, sample.length)
		copy(frame, []byte{255, 251, sample.header, 0})
		path := filepath.Join(t.TempDir(), "audio.mp3")
		data := bytes.Repeat(frame, 1000)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := mp3Quality(file, int64(len(data))); got != sample.want {
			t.Fatalf("actual quality mismatch: %s", got)
		}
		file.Close()
	}
}
func TestLowerActualMP3QualityCannotSkipHigherTarget(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	frame := make([]byte, 417)
	copy(frame, []byte{255, 251, 144, 0})
	source.data = bytes.Repeat(frame, 1000)
	manager, _ := New(store, source, source.load)
	defer manager.Close()
	added, _ := manager.Add(oneSong(), dir, "320k")
	done := awaitTask(t, manager, added[0].ID, "success")
	if done.Quality != "128k" {
		t.Fatal("claimed source quality instead of actual frames")
	}
	added, _ = manager.Add(oneSong(), dir, "320k")
	awaitTask(t, manager, added[0].ID, "success")
}
