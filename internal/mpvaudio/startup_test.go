package mpvaudio

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestStartupObserverReportsFixedStagesIncludingFailure(t *testing.T) {
	for _, reject := range []bool{false, true} {
		f := fake()
		f.rejectInitialize = reject
		var stages []StartupStage
		player, err := openObserved(context.Background(), "http://127.0.0.1:1234/media/test", "flac", func(string) (client, error) { return f, nil }, func(stage StartupStage, duration time.Duration) {
			if duration < 0 {
				t.Fatal("negative elapsed time")
			}
			stages = append(stages, stage)
		})
		expected := []StartupStage{StartupLibrary, StartupInitialize}
		if reject {
			if err == nil || !f.destroyed {
				t.Fatal("failed initialization not cleaned up")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			player.Close()
			expected = append(expected, StartupLoadReady)
		}
		if !reflect.DeepEqual(stages, expected) {
			t.Fatal("incorrect startup timing stages")
		}
	}
}
