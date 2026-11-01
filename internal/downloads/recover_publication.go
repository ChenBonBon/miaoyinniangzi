package downloads

import (
	"context"
	"os"
	"strconv"
	"strings"
)

func directoryStillMounted(root *os.Root, path string) bool {
	original, err := root.Stat(".")
	current, pathErr := os.Stat(path)
	return err == nil && pathErr == nil && os.SameFile(original, current)
}

// A complete, previously verified checkpoint can be published/recovered without
// resolving a fresh signed URL. Hash alone never proves ownership of a name.
func (m *Manager) recoverPublication(ctx context.Context, root *os.Root, r record) (bool, error) {
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	if r.Publication == nil {
		return false, nil
	}
	if !directoryStillMounted(root, r.Task.Directory) {
		return true, ErrDirectoryUnavailable
	}
	if r.Task.File == "" || r.Checkpoint <= 0 || r.Checkpoint != r.Task.Total || r.Digest == "" {
		return true, ErrDownload
	}
	if target, err := regular(root, r.Task.File, true, false); err == nil {
		identity, idErr := identityOf(target)
		if idErr == nil && identity == *r.Publication {
			if syncErr := target.Sync(); syncErr != nil {
				target.Close()
				return true, storageFailure(syncErr)
			}
		}
		target.Close()
		if idErr == nil && identity == *r.Publication {
			if !verified(root, r.Task.File, r.Task.Total, r.Digest) {
				return true, ErrPublishedChanged
			}
			dir, openErr := root.Open(".")
			if openErr != nil {
				return true, storageFailure(openErr)
			}
			syncErr := syncPublishedDirectory(dir)
			dir.Close()
			if syncErr != nil {
				return true, syncErr
			}
			if err = m.finish(ctx, r.Task.ID, r.Task.Total); err != nil {
				return true, err
			}
			// Old hard-link transactions may still have the exact same private name.
			if part, openErr := regular(root, partName(r.Task.ID), false, false); openErr == nil {
				id, _ := identityOf(part)
				part.Close()
				if id == identity {
					_ = root.Remove(partName(r.Task.ID))
				}
			}
			return true, nil
		}
	}
	file, err := regular(root, partName(r.Task.ID), true, false)
	if err != nil {
		return true, ErrPublishedChanged
	}
	defer file.Close()
	identity, err := identityOf(file)
	if err != nil || identity != *r.Publication || !verified(root, partName(r.Task.ID), r.Task.Total, r.Digest) {
		return true, ErrPublishedChanged
	}
	return true, m.publishCompleted(ctx, root, r, file, r.Task.Total)
}
func (m *Manager) publishCompleted(ctx context.Context, root *os.Root, r record, file *os.File, size int64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	identity, err := identityOf(file)
	if err != nil {
		return err
	}
	// Verify the actual local bytes, not just the hash calculated while receiving.
	sum, err := digest(file, size)
	if err != nil {
		return ErrPublishedChanged
	}
	m.mu.Lock()
	current := m.records[r.Task.ID]
	expected := ""
	if current != nil {
		expected = current.Digest
	}
	m.mu.Unlock()
	if sum != expected {
		return ErrPublishedChanged
	}
	base := fileName(r.Task)
	for attempt := 0; attempt < 1000; attempt++ {
		name := base
		if attempt > 0 {
			extension := "." + r.Task.Codec
			name = strings.TrimSuffix(base, extension) + " (" + strconv.Itoa(attempt) + ")" + extension
		}
		if err = m.update(r.Task.ID, func(value *record) { value.Task.File = name; value.Publication = &identity }); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !directoryStillMounted(root, r.Task.Directory) {
			return ErrDirectoryUnavailable
		}
		publisher := m.publish
		if publisher == nil {
			publisher = publishFile
		}
		err = publisher(root, file, identity, partName(r.Task.ID), name)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !verified(root, name, size, expected) {
			return ErrPublishedChanged
		}
		return m.finish(ctx, r.Task.ID, size)
	}
	return ErrDownload
}
