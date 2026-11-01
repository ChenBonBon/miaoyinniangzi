package mediastream

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// CacheFLAC creates one private temporary compressed FLAC file for Core Audio's
// sample-frame decoder. It is not a download record; caller owns cleanup.
func (s *Server) CacheFLAC(ctx context.Context, remote string) (string, func(), error) {
	response, err := s.fetch.OpenMedia(ctx, remote, "GET", "")
	if err != nil {
		return "", nil, ErrMedia
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", nil, ErrMedia
	}
	if _, err := mediaSize(response); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "miaoyin-flac-")
	if err != nil {
		return "", nil, ErrMedia
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "audio.flac")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		cleanup()
		return "", nil, ErrMedia
	}
	count, copyErr := io.CopyBuffer(file, io.LimitReader(response.Body, MaxMediaBytes+1), make([]byte, 32*1024))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || count > MaxMediaBytes || count < 42 || response.ContentLength >= 0 && count != response.ContentLength || ctx.Err() != nil {
		cleanup()
		return "", nil, ErrMedia
	}
	check, err := os.Open(path)
	if err != nil {
		cleanup()
		return "", nil, ErrMedia
	}
	var magic [4]byte
	_, err = io.ReadFull(check, magic[:])
	check.Close()
	if err != nil || string(magic[:]) != "fLaC" {
		cleanup()
		return "", nil, ErrFormat
	}
	return path, cleanup, nil
}
