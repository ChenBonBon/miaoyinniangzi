//go:build !darwin && !linux && !windows

package downloads

import "os"

func identityOf(*os.File) (fileIdentity, error)                    { return fileIdentity{}, ErrPublicationUnsupported }
func renameNoReplace(*os.File, string, string, fileIdentity) error { return ErrPublicationUnsupported }
func syncPublishedDirectory(*os.File) error                        { return ErrPublicationUnsupported }
func platformStorageFailure(error) error                           { return ErrDownload }
