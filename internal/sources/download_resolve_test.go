package sources

import (
	"context"
	"testing"
	"time"

	"miaoyinniangzi/internal/sourceruntime"
)

func TestDownloadAndPlaybackResolutionHaveIndependentCancellation(t *testing.T) {
	m, _, _, _ := fixture(t)
	entered := make(chan string, 4)
	release := make(chan struct{})
	m.checker = &resolverInspector{fakeInspector: fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac"}}}, resolve: func(ctx context.Context, _ []byte, info sourceruntime.ResolveInput, q string) (sourceruntime.Resolution, error) {
		entered <- info.SongMID
		select {
		case <-ctx.Done():
			return sourceruntime.Resolution{}, ctx.Err()
		case <-release:
			return sourceruntime.Resolution{URL: "https://media.example/audio", Quality: q}, nil
		}
	}}
	imported, err := m.Import(context.Background(), script(t, t.TempDir(), "source.js", example))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Activate(context.Background(), imported.Source.ID); err != nil {
		t.Fatal(err)
	}
	playback := make(chan error, 1)
	download := make(chan error, 1)
	go func() {
		_, err := m.Resolve(context.Background(), sourceruntime.ResolveInput{SongMID: "Playback"}, "flac")
		playback <- err
	}()
	<-entered
	go func() {
		_, err := m.ResolveDownload(context.Background(), sourceruntime.ResolveInput{SongMID: "Download"}, "flac")
		download <- err
	}()
	<-entered
	select {
	case <-playback:
		t.Fatal("download canceled playback")
	case <-download:
		t.Fatal("playback canceled download")
	default:
	}
	close(release)
	for _, done := range []chan error{playback, download} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("resolution stalled")
		}
	}
}
