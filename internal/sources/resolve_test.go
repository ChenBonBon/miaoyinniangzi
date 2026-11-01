package sources

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"miaoyinniangzi/internal/sourceruntime"
)

type resolverInspector struct {
	fakeInspector
	resolve func(context.Context, []byte, sourceruntime.ResolveInput, string) (sourceruntime.Resolution, error)
}

func (r *resolverInspector) Resolve(ctx context.Context, script []byte, info sourceruntime.ResolveInput, q string) (sourceruntime.Resolution, error) {
	return r.resolve(ctx, script, info, q)
}
func TestResolveUsesOnlyActiveSourceAndDowngrades(t *testing.T) {
	m, _, _, _ := fixture(t)
	ctx := context.Background()
	calls := []string{}
	inspector := &resolverInspector{fakeInspector: fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac", "320k", "128k"}}}}
	var decrypted []byte
	inspector.resolve = func(_ context.Context, plain []byte, info sourceruntime.ResolveInput, q string) (sourceruntime.Resolution, error) {
		decrypted = plain
		calls = append(calls, q)
		if info.SongMID != "Song1" || len(plain) == 0 {
			t.Fatal("missing snapshot")
		}
		if q == "flac" {
			return sourceruntime.Resolution{Error: "音源未能返回有效地址", RetryLower: true}, nil
		}
		return sourceruntime.Resolution{URL: "https://cdn.example/song", Quality: q}, nil
	}
	m.checker = inspector
	if _, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "flac"); err == nil {
		t.Fatal("resolved without source")
	}
	imported, err := m.Import(ctx, script(t, t.TempDir(), "source.js", example))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Activate(ctx, imported.Source.ID); err != nil {
		t.Fatal(err)
	}
	result, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "flac")
	if err != nil || !result.Downgraded || result.Quality != "320k" || result.SourceID != imported.Source.ID || len(calls) != 2 || calls[0] != "flac" || calls[1] != "320k" {
		t.Fatalf("bad downgrade %+v %v %v", result, calls, err)
	}
	for _, b := range decrypted {
		if b != 0 {
			t.Fatal("decrypted buffer was retained")
		}
	}
	calls = nil
	if _, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "128k"); err != nil || len(calls) != 1 || calls[0] != "128k" {
		t.Fatal("preferred quality was upgraded")
	}
	calls = nil
	result, err = m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "highest")
	if err != nil || result.RequestedQuality != "flac" || result.Quality != "320k" || !result.Downgraded || !reflect.DeepEqual(calls, []string{"flac", "320k"}) {
		t.Fatalf("highest preference failed: %+v %v %v", result, calls, err)
	}
	inspector.resolve = func(_ context.Context, _ []byte, _ sourceruntime.ResolveInput, q string) (sourceruntime.Resolution, error) {
		return sourceruntime.Resolution{URL: "https://cdn.example/song", Quality: q}, nil
	}
	result, err = m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "highest")
	if err != nil || result.RequestedQuality != "flac" || result.Quality != "flac" || result.Downgraded {
		t.Fatal("highest success falsely reported a downgrade")
	}
	for _, known := range []error{sourceruntime.ErrResolveMedia, sourceruntime.ErrResolveMediaDNS, sourceruntime.ErrResolveTimeout, sourceruntime.ErrResolveResources, sourceruntime.ErrResolveProtocol, sourceruntime.ErrResolveProcess} {
		inspector.resolve = func(context.Context, []byte, sourceruntime.ResolveInput, string) (sourceruntime.Resolution, error) {
			return sourceruntime.Resolution{}, known
		}
		_, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "highest")
		if err == nil || err.Error() != known.Error() {
			t.Fatalf("fixed error category lost: %v", err)
		}
	}
	inspector.resolve = func(context.Context, []byte, sourceruntime.ResolveInput, string) (sourceruntime.Resolution, error) {
		return sourceruntime.Resolution{}, errors.New("SECRET URL")
	}
	if _, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "flac"); err == nil || err.Error() == "SECRET URL" {
		t.Fatal("raw error leaked")
	}
}
func TestHighestQualityCandidates(t *testing.T) {
	for _, tc := range []struct {
		preferred       string
		available, want []string
	}{
		{"highest", []string{"128k", "flac", "320k"}, []string{"flac", "320k", "128k"}},
		{"highest", []string{"320k", "128k"}, []string{"320k", "128k"}},
		{"highest", []string{"128k"}, []string{"128k"}},
		{"320k", []string{"flac", "320k", "128k"}, []string{"320k", "128k"}},
		{"highest", []string{"hires"}, []string{}},
	} {
		if got := qualityCandidates(tc.preferred, tc.available); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: got %v want %v", tc.preferred, got, tc.want)
		}
	}
}

func TestSourceMutationCancelsInFlightResolution(t *testing.T) {
	m, _, _, _ := fixture(t)
	ctx := context.Background()
	started := make(chan struct{})
	inspector := &resolverInspector{fakeInspector: fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac"}}}, resolve: func(ctx context.Context, _ []byte, _ sourceruntime.ResolveInput, _ string) (sourceruntime.Resolution, error) {
		close(started)
		<-ctx.Done()
		return sourceruntime.Resolution{}, ctx.Err()
	}}
	m.checker = inspector
	imported, err := m.Import(ctx, script(t, t.TempDir(), "source.js", example))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Activate(ctx, imported.Source.ID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := m.Resolve(ctx, sourceruntime.ResolveInput{SongMID: "Song1"}, "flac")
		finished <- err
	}()
	<-started
	if err := m.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err == nil {
		t.Fatal("stale source result accepted")
	}
}
