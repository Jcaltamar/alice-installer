package envgen

import "os"

// FileWriter writes rendered .env data to the filesystem.
type FileWriter interface {
	WriteEnv(path string, data []byte) error
}

// AtomicWriter implements FileWriter using a write-to-temp-then-rename strategy.
// This ensures that the target file is never partially written.
type AtomicWriter struct{}

// WriteEnv writes data to path atomically via a .tmp sibling file.
// The resulting file has 0600 permissions.
// If the rename fails, the .tmp file is cleaned up before returning the error.
func (AtomicWriter) WriteEnv(path string, data []byte) error {
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return err
	}

	return nil
}

// FakeWriter is a test double for FileWriter.
// It records every write in the Written map (path → data).
// Set Err to simulate errors.
type FakeWriter struct {
	Written map[string][]byte
	Err     error
}

// WriteEnv records the write or returns the configured error.
func (f *FakeWriter) WriteEnv(path string, data []byte) error {
	if f.Err != nil {
		return f.Err
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	f.Written[path] = cp

	return nil
}
