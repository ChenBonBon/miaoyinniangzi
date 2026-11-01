package downloads

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"miaoyinniangzi/internal/mediastream"
)

const chunkBytes int64 = 1024 * 1024

func strong(tag string) bool {
	return len(tag) >= 2 && len(tag) <= 512 && strings.HasPrefix(tag, `"`) && strings.HasSuffix(tag, `"`)
}
func resourceKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.Fragment = ""
	sum := sha256.Sum256([]byte(u.String()))
	return hex.EncodeToString(sum[:])
}
func (m *Manager) exists(root *os.Root, current Task, quality string) bool {
	m.mu.Lock()
	candidates := []record{}
	for _, r := range m.records {
		if r.Task.ID != current.ID && r.Task.State == "success" && r.Task.Song.ID == current.Song.ID && r.Task.Directory == current.Directory && rank(r.Task.Quality) >= rank(quality) {
			candidates = append(candidates, *r)
		}
	}
	m.mu.Unlock()
	for _, r := range candidates {
		if verified(root, r.Task.File, r.Task.Total, r.Digest) {
			return true
		}
	}
	return false
}
func (m *Manager) discardExisting(ctx context.Context, root *os.Root, r record) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	current := m.records[r.Task.ID]
	if current == nil || !active(current.Task.State) || ctx.Err() != nil {
		m.mu.Unlock()
		return context.Canceled
	}
	if err := m.store.RemoveDownload(context.Background(), r.Task.ID); err != nil {
		m.mu.Unlock()
		return ErrDownload
	}
	delete(m.records, r.Task.ID)
	notice := m.onDuplicate
	m.mu.Unlock()
	if r.Checkpoint > 0 && verified(root, partName(r.Task.ID), r.Checkpoint, r.Digest) {
		_ = root.Remove(partName(r.Task.ID))
	}
	if notice != nil {
		notice()
	}
	return nil
}
func (m *Manager) run(ctx context.Context, r record) error {
	root, err := os.OpenRoot(r.Task.Directory)
	if err != nil {
		classified := storageFailure(err)
		if classified == ErrDownload {
			return ErrDirectoryUnavailable
		}
		return classified
	}
	defer root.Close()
	if recovered, recoveryErr := m.recoverPublication(ctx, root, r); recovered {
		return recoveryErr
	}
	if m.exists(root, r.Task, r.Task.Requested) {
		return m.discardExisting(ctx, root, r)
	}
	preferred := r.Task.Requested
	if r.Checkpoint > 0 && rank(r.Task.Quality) > 0 {
		preferred = r.Task.Quality
	}
	media, err := m.load(ctx, r.Task.Song, preferred)
	if err != nil || ctx.Err() != nil {
		return ErrDownload
	}
	if rank(media.Quality) == 0 || media.Info.Size <= 0 || media.Info.Size > mediastream.MaxMediaBytes || media.Info.Codec != "flac" && media.Info.Codec != "mp3" || media.Quality == "flac" && media.Info.Codec != "flac" || media.Quality != "flac" && media.Info.Codec != "mp3" {
		return ErrDownload
	}
	if m.exists(root, r.Task, media.Quality) {
		return m.discardExisting(ctx, root, r)
	}
	file, err := regular(root, partName(r.Task.ID), true, r.Checkpoint == 0)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return ErrDownload
	}
	if r.Checkpoint > 0 {
		if stat.Size() < r.Checkpoint {
			return ErrDownload
		}
		previous, err := digest(file, r.Checkpoint)
		if err != nil || previous != r.Digest {
			return ErrDownload
		}
	}
	// Recover a crash between no-replace publication and the final database write.
	if r.Task.File != "" && r.Checkpoint > 0 && r.Checkpoint == r.Task.Total && verified(root, r.Task.File, r.Checkpoint, r.Digest) {
		target, statErr := root.Lstat(r.Task.File)
		if statErr == nil && os.SameFile(stat, target) {
			if err = m.finish(ctx, r.Task.ID, r.Checkpoint); err != nil {
				return err
			}
			_ = root.Remove(partName(r.Task.ID))
			return nil
		}
	}
	resumable := r.Checkpoint > 0 && strong(media.Info.EntityTag) && media.Info.Seekable && r.ETag == media.Info.EntityTag && r.Resource == resourceKey(media.URL) && r.Source == media.Source && r.Task.Total == media.Info.Size && r.Task.Quality == media.Quality && r.Task.Codec == media.Info.Codec
	offset := int64(0)
	if resumable {
		offset = r.Checkpoint
	}
	if err = file.Truncate(offset); err != nil {
		return storageFailure(err)
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return storageFailure(err)
	}
	rolling := sha256.New()
	if offset > 0 {
		if _, err = io.Copy(rolling, io.NewSectionReader(file, 0, offset)); err != nil {
			return ErrDownload
		}
	}
	message := ""
	if r.Checkpoint > 0 && !resumable {
		message = "无法确认同一媒体实体，已从头下载"
	}
	if media.Downgraded {
		message += " 已降至 " + strings.ToUpper(media.Quality)
	}
	if err = m.update(r.Task.ID, func(v *record) {
		if ctx.Err() != nil || !active(v.Task.State) {
			return
		}
		v.Task.State = "downloading"
		v.Task.Quality = media.Quality
		v.Task.Codec = media.Info.Codec
		v.Task.Total = media.Info.Size
		v.Task.Bytes = offset
		v.Task.File = ""
		v.Publication = nil
		v.Task.Message = strings.TrimSpace(message)
		v.Checkpoint = offset
		v.Digest = hex.EncodeToString(rolling.Sum(nil))
		v.ETag = media.Info.EntityTag
		v.Resource = resourceKey(media.URL)
		v.Source = media.Source
	}); err != nil {
		return err
	}
	for offset < media.Info.Size {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		end := media.Info.Size - 1
		useRange := strong(media.Info.EntityTag) && media.Info.Seekable
		if useRange && end-offset+1 > chunkBytes {
			end = offset + chunkBytes - 1
		}
		requested := ""
		if useRange {
			requested = fmt.Sprintf("bytes=%d-%d", offset, end)
		}
		response, err := m.raw.OpenMedia(ctx, media.URL, "GET", requested)
		if err != nil {
			return ErrDownload
		}
		expected := end - offset + 1
		valid := response.StatusCode == http.StatusOK && offset == 0 && end == media.Info.Size-1 || response.StatusCode == http.StatusPartialContent && response.Header.Get("Content-Range") == fmt.Sprintf("bytes %d-%d/%d", offset, end, media.Info.Size)
		if !valid || response.ContentLength >= 0 && response.ContentLength != expected || strong(media.Info.EntityTag) && response.Header.Get("ETag") != media.Info.EntityTag {
			response.Body.Close()
			return ErrDownload
		}
		n, copyErr := m.copy(ctx, r.Task.ID, file, rolling, response.Body, offset, expected, media.Info.Codec)
		response.Body.Close()
		offset += n
		if n > 0 {
			if err = file.Sync(); err != nil {
				return storageFailure(err)
			}
			if err = m.update(r.Task.ID, func(v *record) {
				v.Task.Bytes = offset
				v.Checkpoint = offset
				v.Digest = hex.EncodeToString(rolling.Sum(nil))
			}); err != nil {
				return err
			}
		}
		if copyErr != nil {
			return copyErr
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r.Task.Quality = media.Quality
	r.Task.Codec = media.Info.Codec
	if media.Info.Codec == "mp3" {
		actual := mp3Quality(file, media.Info.Size)
		if actual != media.Quality {
			r.Task.Quality = actual
			if err = m.update(r.Task.ID, func(v *record) {
				v.Task.Quality = actual
				v.Task.Message = "实际 MP3 码率与音源档位不一致，已按实际文件保存"
			}); err != nil {
				return err
			}
		}
	}
	return m.publishCompleted(ctx, root, r, file, media.Info.Size)
}
func (m *Manager) finish(ctx context.Context, id string, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.records[id]
	if ctx.Err() != nil || r == nil || !active(r.Task.State) {
		return context.Canceled
	}
	before := *r
	r.Task.State = "success"
	r.Task.Bytes = size
	if err := m.saveLocked(r); err != nil {
		*r = before
		return err
	}
	return nil
}
func (m *Manager) copy(ctx context.Context, id string, file *os.File, rolling hash.Hash, body io.Reader, offset, expected int64, codec string) (int64, error) {
	buffer := make([]byte, 64*1024)
	var written int64
	updated := time.Now()
	if offset == 0 {
		reader := bufio.NewReaderSize(body, 4096)
		size := int64(4096)
		if expected < size {
			size = expected
		}
		head, err := reader.Peek(int(size))
		if err != nil && err != io.EOF {
			return 0, ErrDownload
		}
		if codec == "flac" && (len(head) < 4 || string(head[:4]) != "fLaC") || codec == "mp3" && !mp3Header(head) {
			return 0, ErrDownload
		}
		body = reader
	}
	for written < expected {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		size := int64(len(buffer))
		if expected-written < size {
			size = expected - written
		}
		n, err := body.Read(buffer[:size])
		if n > 0 {
			count, writeErr := file.Write(buffer[:n])
			if count > 0 {
				_, _ = rolling.Write(buffer[:count])
				written += int64(count)
			}
			if writeErr != nil {
				return written, storageFailure(writeErr)
			}
			if count != n {
				return written, ErrDiskFull
			}
			if time.Since(updated) > 250*time.Millisecond {
				_ = m.update(id, func(v *record) { v.Task.Bytes = offset + written })
				updated = time.Now()
			}
		}
		if err != nil {
			if err == io.EOF && written == expected {
				return written, nil
			}
			return written, ErrDownload
		}
		if n == 0 {
			return written, ErrDownload
		}
	}
	var extra [1]byte
	n, err := body.Read(extra[:])
	if n != 0 || err != io.EOF {
		return written, ErrDownload
	}
	return written, nil
}
func mp3Header(data []byte) bool {
	if len(data) >= 3 && string(data[:3]) == "ID3" {
		return true
	}
	for i := 0; i+4 <= len(data) && i < 1024; i++ {
		if data[i] == 255 && data[i+1]&224 == 224 && data[i+1]&6 == 2 && data[i+2]&240 != 0 && data[i+2]&240 != 240 {
			return true
		}
	}
	return false
}
