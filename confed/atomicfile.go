package confed

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/google/renameio/v2"
)

// Resolve the final symlink, including links to files that do not exist yet.
// Renaming over the link itself would disconnect it from its intended target.
func resolveConfigPath(path string) (string, error) {
	for links := 0; ; links++ {
		dir, name := filepath.Split(path)
		if dir == "" {
			dir = "."
		}
		dir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		if links >= 40 {
			return "", fmt.Errorf("resolve config path %s: %w", path, syscall.ELOOP)
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(target) {
			path = target
		} else {
			// Keep '..' until the next directory resolution: cleaning it before
			// following directory symlinks could select a different target.
			path = dir + string(os.PathSeparator) + target
		}
	}
}

// writeConfigFileAtomic leaves the old file intact until the complete replacement
// has been written, synced and closed. Existing readers keep the old inode.
// renameio manages the temporary file and atomic replacement; this adapter adds
// symlink resolution, ownership preservation and directory syncing for durability.
func writeConfigFileAtomic(path string, content io.Reader) error {
	path, err := resolveConfigPath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if info != nil && !info.Mode().IsRegular() {
		return fmt.Errorf("config path %s is not a regular file", path)
	}

	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()

	// Keep the previous creation permissions (subject to umask) for new files.
	// Existing files' replacements stay private until their metadata is restored.
	mode := os.FileMode(0777)
	if info != nil {
		mode = 0600
	}
	temp, err := renameio.NewPendingFile(path,
		renameio.WithTempDir(filepath.Dir(path)),
		renameio.WithPermissions(mode),
	)
	if err != nil {
		return err
	}
	defer temp.Cleanup()

	if _, err = io.Copy(temp, content); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
	}
	if info != nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot read ownership of %s", path)
		}
		if err = temp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return fmt.Errorf("preserve config ownership: %w", err)
		}
		// Chown and writes can clear set-ID bits, so restore the mode last.
		if err = temp.Chmod(info.Mode()); err != nil {
			return fmt.Errorf("preserve config permissions: %w", err)
		}
	}
	if err = temp.CloseAtomicallyReplace(); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	// The rename has committed at this point; sync its directory entry so the
	// replacement is durable before the caller schedules a service restart.
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
}
