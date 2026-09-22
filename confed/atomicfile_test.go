package confed

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"
)

func checkFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("content of %s = %q, want %q", path, content, expected)
	}
}

func checkNoTemporaryConfigs(t *testing.T, dir string) {
	t.Helper()
	// Test fixtures have no hidden files; catch temporary files regardless of
	// the naming scheme used by the atomic-write implementation.
	paths, err := filepath.Glob(filepath.Join(dir, ".*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("temporary config files were not removed: %v", paths)
	}
}

func TestAtomicConfigReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()

	if err := writeConfigFileAtomic(path, strings.NewReader("new config")); err != nil {
		t.Fatal(err)
	}
	checkFileContent(t, path, "new config")
	// A truncating write would also modify this already-open file descriptor.
	content, err := io.ReadAll(old)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old config" {
		t.Fatalf("existing reader sees %q, want old config", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("file permissions = %o, want 640", info.Mode().Perm())
	}
	checkNoTemporaryConfigs(t, dir)
}

func TestAtomicConfigWriteFailure(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new file"
		if existing {
			name = "existing file"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if existing {
				if err := os.WriteFile(path, []byte("old config"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("interrupted input")
			content := io.MultiReader(strings.NewReader("partial replacement"), iotest.ErrReader(failure))
			if err := writeConfigFileAtomic(path, content); !errors.Is(err, failure) {
				t.Fatalf("write error = %v, want %v", err, failure)
			}
			if existing {
				checkFileContent(t, path, "old config")
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("failed write created destination: %v", err)
			}
			checkNoTemporaryConfigs(t, dir)
		})
	}
}

func TestAtomicConfigCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeConfigFileAtomic(path, strings.NewReader("new config")); err != nil {
		t.Fatal(err)
	}
	checkFileContent(t, path, "new config")
	// Compare against the previous creation behavior without changing umask.
	reference := filepath.Join(dir, "reference.json")
	if err := os.WriteFile(reference, nil, 0777); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(reference)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != want.Mode() {
		t.Fatalf("file permissions = %v, want %v", info.Mode(), want.Mode())
	}
	checkNoTemporaryConfigs(t, dir)
}

func TestAtomicConfigSymlink(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "dangling"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "storage", "nested"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join("storage", "nested"), filepath.Join(dir, "linked-dir")); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "storage", "target.json")
			if existing {
				if err := os.WriteFile(target, []byte("old config"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			link := filepath.Join(dir, "config.json")
			if err := os.Symlink("linked-dir/../target.json", link); err != nil {
				t.Fatal(err)
			}
			// Cover both absolute and relative links in the same chain.
			outerLink := filepath.Join(dir, "outer.json")
			if err := os.Symlink(link, outerLink); err != nil {
				t.Fatal(err)
			}
			if err := writeConfigFileAtomic(outerLink, strings.NewReader("new config")); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{link, outerLink} {
				if _, err := os.Readlink(path); err != nil {
					t.Fatalf("symlink %s was replaced: %v", path, err)
				}
			}
			checkFileContent(t, target, "new config")
			checkNoTemporaryConfigs(t, dir)
			checkNoTemporaryConfigs(t, filepath.Dir(target))
		})
	}
}

func TestAtomicConfigSymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.Symlink("config.json", path); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFileAtomic(path, strings.NewReader("new config")); !errors.Is(err, syscall.ELOOP) {
		t.Fatalf("write error = %v, want ELOOP", err)
	}
	if _, err := os.Readlink(path); err != nil {
		t.Fatalf("symlink was replaced: %v", err)
	}
	checkNoTemporaryConfigs(t, dir)
}

func TestAtomicConfigPreservesOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing file ownership requires root")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("old config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 1234, 2345); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFileAtomic(path, strings.NewReader("new config")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if stat.Uid != 1234 || stat.Gid != 2345 {
		t.Fatalf("file owner = %d:%d, want 1234:2345", stat.Uid, stat.Gid)
	}
}
