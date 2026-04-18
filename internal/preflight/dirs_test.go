package preflight_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

func TestOSDirChecker_WritableDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // always writable

	checker := preflight.OSDirChecker{}
	ok, reason := checker.IsWritable(dir)
	if !ok {
		t.Errorf("IsWritable(%q) = false, reason=%q; want true", dir, reason)
	}
	if reason != "" {
		t.Errorf("IsWritable(%q) reason = %q, want empty", dir, reason)
	}
}

func TestOSDirChecker_NonExistentSubdir_ParentWritable(t *testing.T) {
	t.Parallel()
	// Parent is a writable temp dir; child doesn't exist yet.
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir-that-does-not-exist")

	checker := preflight.OSDirChecker{}
	ok, _ := checker.IsWritable(child)
	// Parent is writable, so we should be able to create the child.
	if !ok {
		t.Errorf("IsWritable(%q) = false, want true (parent writable)", child)
	}
}

func TestOSDirChecker_NonExistentSubdir_ParentNotExist(t *testing.T) {
	t.Parallel()
	// Neither parent nor child exists.
	path := filepath.Join(t.TempDir(), "ghost", "nested")

	checker := preflight.OSDirChecker{}
	ok, reason := checker.IsWritable(path)
	if ok {
		t.Errorf("IsWritable(%q) = true, want false (parent doesn't exist)", path)
	}
	if reason == "" {
		t.Errorf("IsWritable(%q): reason must not be empty when returning false", path)
	}
}

func TestOSDirChecker_ReadOnlyDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chmod test in -short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("chmod 0555 test only meaningful on Linux")
	}

	t.Parallel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so TempDir cleanup works.
		_ = os.Chmod(dir, 0755)
	})

	checker := preflight.OSDirChecker{}
	ok, reason := checker.IsWritable(dir)
	if ok {
		t.Errorf("IsWritable(%q) = true, want false (dir is 0555)", dir)
	}
	if reason == "" {
		t.Errorf("IsWritable(%q): reason must not be empty when returning false", dir)
	}
}
