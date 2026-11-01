package playbackdiag

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func flush(t *testing.T, d *Diagnostics) {
	t.Helper()
	d.Close()
	select {
	case <-d.done:
	case <-time.After(2 * time.Second):
		t.Fatal("diagnostics writer did not finish")
	}
}
func TestTraceContainsOnlyFixedNamesAndNumbers(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	trace := Begin(d, 7, time.Now().Add(-100*time.Millisecond))
	trace.Format("flac", "flac")
	trace.Format("SECRET-SONG", "https://secret.invalid/signed?key=SECRET-KEY")
	trace.Add(Resolve, 12*time.Millisecond)
	trace.Add(Stage(255), time.Second)
	trace.Add(Probe, -time.Second)
	trace.Finish(errors.New("SECRET-ERROR"))
	trace.FirstPlay(time.Now().Add(-2*time.Millisecond), nil, true, false)
	trace.FirstPlay(time.Now(), nil, true, false)
	flush(t, d)
	data, err := os.ReadFile(filepath.Join(dir, "playback.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "https:") {
		t.Fatal("private strings reached disk")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatal("expected prepare and only the first unpause")
	}
	var prepared entry
	if json.Unmarshal([]byte(lines[0]), &prepared) != nil || prepared.Sequence != 7 || prepared.Result != "failed" || prepared.Codec != "flac" || len(prepared.Stages) != 1 || prepared.Stages[0].DurationMS != 12 {
		t.Fatal("invalid safe trace")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "playback.jsonl"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("log permissions")
		}
	}
	// Logging after closure must be a harmless no-op.
	trace.Finish(nil)
}
func TestResultClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		name string
	}{{nil, "success"}, {context.Canceled, "canceled"}, {context.DeadlineExceeded, "timeout"}, {errors.New("secret"), "failed"}} {
		if outcome(tc.err) != tc.name {
			t.Fatal("unexpected result label")
		}
	}
}
func TestRotationAndUnsafeDestinations(t *testing.T) {
	dir := t.TempDir()
	d := &Diagnostics{directory: dir}
	event := entry{Time: "fixed", Operation: "prepare", Result: "success", Stages: []timing{{"resolve", 10}}}
	for range 2000 {
		d.write(event)
	}
	for _, name := range []string{"playback.jsonl", "playback.jsonl.1"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() > limit {
			t.Fatal("unbounded log")
		}
	}
	root := t.TempDir()
	outside := filepath.Join(root, "private")
	if os.WriteFile(outside, []byte("UNCHANGED"), 0600) != nil {
		t.Fatal("fixture")
	}
	unsafeDir := filepath.Join(root, "logs")
	if os.Mkdir(unsafeDir, 0700) != nil {
		t.Fatal("fixture")
	}
	if err := os.Symlink(outside, filepath.Join(unsafeDir, "playback.jsonl")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	(&Diagnostics{directory: unsafeDir}).write(event)
	after, _ := os.ReadFile(outside)
	if string(after) != "UNCHANGED" {
		t.Fatal("followed log symlink")
	}
}
func TestUnavailableLoggingDoesNotFailOrBlockProducer(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "not-directory")
	if os.WriteFile(path, []byte("x"), 0600) != nil {
		t.Fatal("fixture")
	}
	d := New(path)
	for i := range 1000 {
		trace := Begin(d, uint64(i), time.Now())
		trace.Finish(nil)
	}
	flush(t, d)
}
