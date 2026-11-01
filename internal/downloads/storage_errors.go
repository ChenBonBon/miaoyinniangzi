package downloads

import "errors"

func storageFailure(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrDiskFull, ErrDirectoryUnavailable, ErrDirectoryPermission, ErrPublicationUnsupported, ErrPublishedChanged} {
		if errors.Is(err, known) {
			return known
		}
	}
	return platformStorageFailure(err)
}
func failureMessage(err error) string {
	for _, known := range []error{ErrDiskFull, ErrDirectoryUnavailable, ErrDirectoryPermission, ErrPublicationUnsupported, ErrPublishedChanged} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrDownload.Error()
}
