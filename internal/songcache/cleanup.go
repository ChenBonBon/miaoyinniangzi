package songcache

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var cleanupOnce sync.Once

func cleanupAbandoned() {
	paths, _ := filepath.Glob(filepath.Join(os.TempDir(), "miaoyin-song-cache-*"))
	if len(paths) > 1000 {
		paths = paths[:1000]
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			continue
		}
		marker, err := root.Open("owner.pid")
		if err != nil {
			root.Close()
			continue
		}
		data := make([]byte, 33)
		n, readErr := marker.Read(data)
		marker.Close()
		root.Close()
		if readErr != nil || n == 0 || n > 32 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data[:n])))
		if err != nil || pid <= 0 || processAlive(pid) {
			continue
		}
		_ = os.RemoveAll(path)
	}
}
