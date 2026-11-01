package songcache

import (
	"context"
	"testing"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/mpvaudio"
)

func TestMissingStrongValidatorFallsBackWithoutClaimingCompleteCache(t *testing.T) {
	source := &memorySource{data: fakeMP3(12)}
	m := New(context.Background(), source)
	defer m.Close()
	m.open = func(context.Context, string, string) (*mpvaudio.Player, error) { return &mpvaudio.Player{}, nil }
	request := Request{Key: Key{Source: "source", Quality: "128k", Song: "one"}, Load: func(context.Context) (Spec, error) {
		return Spec{Remote: "http://public.example/audio", Info: mediastream.Info{Codec: "mp3", Size: int64(len(source.data)), Seekable: true}}, nil
	}}
	player, spec, _, err := m.Use(context.Background(), 1, request, nil)
	if err != nil {
		t.Fatal("unsupported cache broke streaming")
	}
	defer player.Close()
	if spec.Cacheable {
		t.Fatal("unvalidated ranges advertised as cacheable")
	}
	m.mu.Lock()
	disk := m.entries[request.Key].disk
	m.mu.Unlock()
	if disk != nil {
		t.Fatal("unvalidated partial bytes retained")
	}
}
