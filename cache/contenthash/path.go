package contenthash

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

var errTooManyLinks error = unix.ELOOP

const maxSymlinkLimit = 255

type onSymlinkFunc func(string, string) error

// rootPath joins a path with a root, evaluating and bounding any
// symlink to the root directory.
// This is github.com/cyphar/filepath-securejoin.SecureJoinVFS's implementation
// with a callback on resolving the symlink.
func rootPath(root, unsafePath string, cb onSymlinkFunc) (string, error) {
	if unsafePath == "" {
		return root, nil
	}
	unsafePath = filepath.FromSlash(unsafePath)

	var (
		path        string
		linksWalked int
	)
	for unsafePath != "" {
		// Windows-specific: remove any drive letters from the path.
		if v := filepath.VolumeName(unsafePath); v != "" {
			unsafePath = unsafePath[len(v):]
		}

		// Get the next path component.
		var part string
		if i := strings.IndexRune(unsafePath, filepath.Separator); i == -1 {
			part, unsafePath = unsafePath, ""
		} else {
			part, unsafePath = unsafePath[:i], unsafePath[i+1:]
		}

		// Apply the component lexically to the path we are building. path does
		// not contain any symlinks, and we are lexically dealing with a single
		// component, so it's okay to do filepath.Clean here.
		nextPath := filepath.Join(string(filepath.Separator), path, part)
		if nextPath == string(filepath.Separator) {
			// If we end up back at the root, we don't need to re-evaluate /.
			path = "/"
			continue
		}
		fullPath := filepath.Join(root, nextPath)

		// Figure out whether the path is a symlink.
		fi, err := os.Lstat(fullPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		// Treat non-existent path components the same as non-symlinks (we
		// can't do any better here).
		if errors.Is(err, os.ErrNotExist) || fi.Mode()&os.ModeSymlink == 0 {
			path = nextPath
			continue
		}

		// It's a symlink, so get its contents and expand it by prepending it
		// to the yet-unparsed path.
		linksWalked++
		if linksWalked > maxSymlinkLimit {
			return "", errTooManyLinks
		}

		dest, err := os.Readlink(fullPath)
		if err != nil {
			return "", err
		}
		if cb != nil {
			if err := cb(nextPath, dest); err != nil {
				return "", err
			}
		}
		unsafePath = dest + string(filepath.Separator) + unsafePath
		// Absolute symlinks reset any work we've already done.
		if filepath.IsAbs(dest) {
			path = "/"
		}
	}

	// There should be no lexical components left in path here, but just for
	// safety do a filepath.Clean before the join.
	finalPath := filepath.Join(string(filepath.Separator), path)
	return filepath.Join(root, finalPath), nil
}
