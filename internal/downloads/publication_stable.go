//go:build !windows

package downloads

import "os"

func renameNoReplaceIdentity(dir *os.File, source, target string, expected fileIdentity) (fileIdentity, error) {
	if err := renameNoReplace(dir, source, target, expected); err != nil {
		return fileIdentity{}, err
	}
	return expected, nil
}
