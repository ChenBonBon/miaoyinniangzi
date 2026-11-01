package songcache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/privatefs"
)

const chunkSize int64 = 256 * 1024

type span struct{ start, end int64 }
type fetchJob struct {
	done chan struct{}
	data []byte
	err  error
}
type diskFile struct {
	mu                   sync.Mutex
	raw                  mediastream.Fetcher
	remote               string
	info                 mediastream.Info
	file                 *os.File
	directory            string
	ctx                  context.Context
	cancel               context.CancelFunc
	slots                *budget
	spans                []span
	jobs                 map[int64]*fetchJob
	full, closed, failed bool
	prefix               int64
	etag                 string
	background           context.CancelFunc
}

func newDisk(ctx context.Context, raw mediastream.Fetcher, remote string, info mediastream.Info, slots *budget) (*diskFile, error) {
	if info.Size <= 0 || info.Size > mediastream.MaxMediaBytes || !info.Seekable {
		return nil, ErrCache
	}
	directory, err := os.MkdirTemp("", "miaoyin-song-cache-")
	if err != nil {
		return nil, ErrCache
	}
	if err := privatefs.Protect(directory, 0700); err != nil {
		os.RemoveAll(directory)
		return nil, ErrCache
	}
	if err := os.WriteFile(directory+"/owner.pid", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.RemoveAll(directory)
		return nil, ErrCache
	}
	file, err := os.OpenFile(directory+"/audio.cache", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		os.RemoveAll(directory)
		return nil, ErrCache
	}
	own, cancel := context.WithCancel(ctx)
	return &diskFile{raw: raw, remote: remote, info: info, file: file, directory: directory, ctx: own, cancel: cancel, slots: slots, jobs: map[int64]*fetchJob{}}, nil
}
func (d *diskFile) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	d.cancel()
	if d.background != nil {
		d.background()
	}
	_ = d.file.Close()
	_ = os.RemoveAll(d.directory)
}
func (d *diskFile) available(offset int64) int64 {
	for _, s := range d.spans {
		if s.start <= offset && offset < s.end {
			return s.end - offset
		}
	}
	return 0
}
func (d *diskFile) add(start, end int64) {
	d.spans = append(d.spans, span{start, end})
	sort.Slice(d.spans, func(i, j int) bool { return d.spans[i].start < d.spans[j].start })
	merged := d.spans[:0]
	for _, s := range d.spans {
		if len(merged) > 0 && s.start <= merged[len(merged)-1].end {
			if s.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = s.end
			}
		} else {
			merged = append(merged, s)
		}
	}
	d.spans = merged
}
func (d *diskFile) trimLocked() {
	if d.prefix <= 0 {
		return
	}
	clipped := []span{}
	for _, s := range d.spans {
		if s.start >= d.prefix {
			continue
		}
		if s.end > d.prefix {
			s.end = d.prefix
		}
		clipped = append(clipped, s)
	}
	d.spans = clipped
	if d.file.Truncate(d.prefix) != nil {
		d.failed = true
	}
}
func (d *diskFile) setFull(full bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.full == full {
		return
	}
	if d.background != nil {
		d.background()
		d.background = nil
	}
	d.full = full
	if !full {
		if d.prefix > 0 {
			d.trimLocked()
		} else {
			d.spans = nil
			if d.file.Truncate(0) != nil {
				d.failed = true
			}
		}
		return
	}
	ctx, cancel := context.WithCancel(d.ctx)
	d.background = cancel
	go func() {
		var first bytes.Buffer
		for offset := int64(0); offset < d.info.Size; offset += chunkSize {
			if ctx.Err() != nil {
				return
			}
			end := offset + chunkSize
			if end > d.info.Size {
				end = d.info.Size
			}
			buf := make([]byte, int(end-offset))
			if _, err := d.readAt(ctx, buf, offset); err != nil {
				return
			}
			d.mu.Lock()
			known := d.prefix > 0
			d.mu.Unlock()
			if !known && first.Len() < maxPrefix {
				first.Write(buf)
				if cut, err := prefixEnd(first.Bytes(), d.info.Codec, 5); err == nil {
					d.mu.Lock()
					d.prefix = int64(cut)
					if !d.full {
						d.trimLocked()
					}
					d.mu.Unlock()
					first.Reset()
				}
			}
		}
	}()
}
func (d *diskFile) preparePrefix(ctx context.Context) error {
	var data bytes.Buffer
	for offset := int64(0); offset < d.info.Size && offset < maxPrefix; offset += chunkSize {
		end := offset + chunkSize
		if end > d.info.Size {
			end = d.info.Size
		}
		buf := make([]byte, int(end-offset))
		if _, err := d.readAt(ctx, buf, offset); err != nil {
			return err
		}
		data.Write(buf)
		cut, err := prefixEnd(data.Bytes(), d.info.Codec, 5)
		if err == errMore && end == d.info.Size {
			cut = int(d.info.Size)
			err = nil
		}
		if err == nil {
			d.mu.Lock()
			d.prefix = int64(cut)
			if !d.full {
				d.trimLocked()
			}
			d.mu.Unlock()
			return nil
		}
		if err != errMore {
			return ErrCache
		}
	}
	return ErrCache
}
func (d *diskFile) readAt(ctx context.Context, p []byte, offset int64) (int, error) {
	if offset >= d.info.Size {
		return 0, io.EOF
	}
	total := 0
	for len(p) > 0 && offset < d.info.Size {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return total, context.Canceled
		}
		available := d.available(offset)
		if available > 0 {
			n := int64(len(p))
			if n > available {
				n = available
			}
			if n > d.info.Size-offset {
				n = d.info.Size - offset
			}
			got, err := d.file.ReadAt(p[:int(n)], offset)
			d.mu.Unlock()
			total += got
			offset += int64(got)
			p = p[got:]
			if err != nil {
				return total, err
			}
			continue
		}
		start := offset / chunkSize * chunkSize
		job := d.jobs[start]
		if job == nil {
			job = &fetchJob{done: make(chan struct{})}
			d.jobs[start] = job
			go d.fetch(start, job, d.full && ctx.Value(playbackReadKey{}) == true)
		}
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-d.ctx.Done():
			return total, context.Canceled
		case <-job.done:
		}
		if job.err != nil {
			return total, job.err
		}
		index := int(offset - start)
		n := copy(p, job.data[index:])
		if n == 0 {
			return total, ErrCache
		}
		total += n
		offset += int64(n)
		p = p[n:]
	}
	if len(p) > 0 {
		return total, io.EOF
	}
	return total, nil
}
func (d *diskFile) fetch(start int64, job *fetchJob, foreground bool) {
	data, etag, err := d.fetchRange(start, foreground)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		err = context.Canceled
	}
	if err == nil && etag != "" && d.etag != "" && d.etag != etag {
		err = ErrCache
	}
	if err == nil {
		if d.etag == "" {
			d.etag = etag
		}
		end := start + int64(len(data))
		storeEnd := end
		if !d.full {
			limit := d.prefix
			if limit == 0 {
				limit = maxPrefix
			}
			if storeEnd > limit {
				storeEnd = limit
			}
		}
		if storeEnd > start {
			if _, e := d.file.WriteAt(data[:int(storeEnd-start)], start); e != nil {
				err = ErrCache
			} else {
				d.add(start, storeEnd)
			}
		}
	}
	if err != nil && !d.closed {
		d.failed = true
	}
	job.data = data
	job.err = err
	delete(d.jobs, start)
	close(job.done)
}
func (d *diskFile) fetchRange(start int64, foreground bool) ([]byte, string, error) {
	release, err := d.slots.acquire(d.ctx, foreground)
	if err != nil {
		return nil, "", err
	}
	defer release()
	end := start + chunkSize
	if end > d.info.Size {
		end = d.info.Size
	}
	response, err := d.raw.OpenMedia(d.ctx, d.remote, "GET", fmt.Sprintf("bytes=%d-%d", start, end-1))
	if err != nil {
		return nil, "", ErrCache
	}
	defer response.Body.Close()
	if response.StatusCode != 206 || response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end-1, d.info.Size) {
		return nil, "", ErrCache
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, end-start+1))
	if err != nil || int64(len(data)) != end-start {
		return nil, "", ErrCache
	}
	etag := response.Header.Get("ETag")
	if len(etag) < 2 || len(etag) > 512 || !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		return nil, "", ErrCache
	}
	return data, etag, nil
}
func (d *diskFile) OpenMedia(ctx context.Context, remote, method, rangeValue string) (*http.Response, error) {
	if remote != d.remote {
		return nil, ErrCache
	}
	start, end := int64(0), d.info.Size-1
	status := 200
	if rangeValue != "" {
		if !strings.HasPrefix(rangeValue, "bytes=") || strings.Contains(rangeValue, ",") {
			return nil, ErrCache
		}
		parts := strings.Split(strings.TrimPrefix(rangeValue, "bytes="), "-")
		if len(parts) != 2 {
			return nil, ErrCache
		}
		var err error
		if parts[0] == "" {
			count, e := strconv.ParseInt(parts[1], 10, 64)
			if e != nil || count <= 0 {
				return nil, ErrCache
			}
			start = d.info.Size - count
			if start < 0 {
				start = 0
			}
		} else {
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				return nil, ErrCache
			}
			if parts[1] != "" {
				end, err = strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					return nil, ErrCache
				}
			}
		}
		if start < 0 || start >= d.info.Size || end < start {
			h := make(http.Header)
			h.Set("Content-Range", fmt.Sprintf("bytes */%d", d.info.Size))
			return &http.Response{StatusCode: 416, Header: h, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
		if end >= d.info.Size {
			end = d.info.Size - 1
		}
		status = 206
	}
	header := make(http.Header)
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if status == 206 {
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, d.info.Size))
	}
	var body io.ReadCloser = io.NopCloser(bytes.NewReader(nil))
	if method != "HEAD" {
		own, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(d.ctx, cancel)
		body = &cacheBody{disk: d, ctx: context.WithValue(own, playbackReadKey{}, true), offset: start, end: end + 1, cancel: func() { stop(); cancel() }}
	}
	return &http.Response{StatusCode: status, Header: header, ContentLength: end - start + 1, Body: body}, nil
}

type cacheBody struct {
	disk        *diskFile
	ctx         context.Context
	offset, end int64
	cancel      context.CancelFunc
	buffer      []byte
}

func (b *cacheBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.buffer) == 0 {
		if b.offset >= b.end {
			return 0, io.EOF
		}
		size := chunkSize - b.offset%chunkSize
		if size > b.end-b.offset {
			size = b.end - b.offset
		}
		buffer := make([]byte, int(size))
		n, err := b.disk.readAt(b.ctx, buffer, b.offset)
		if err != nil && n == 0 {
			return 0, err
		}
		b.buffer = buffer[:n]
	}
	n := copy(p, b.buffer)
	b.buffer = b.buffer[n:]
	b.offset += int64(n)
	return n, nil
}
func (b *cacheBody) Close() error { b.cancel(); return nil }
