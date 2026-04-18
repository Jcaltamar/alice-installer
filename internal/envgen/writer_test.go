package envgen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/envgen"
)

func TestAtomicWriter_WriteEnv(t *testing.T) {
	t.Run("creates file with correct content", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		data := []byte("POSTGRES_PASSWORD=secret\n")

		w := envgen.AtomicWriter{}
		if err := w.WriteEnv(path, data); err != nil {
			t.Fatalf("WriteEnv() error: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile after write: %v", err)
		}

		if string(got) != string(data) {
			t.Errorf("content mismatch: got %q, want %q", got, data)
		}
	})

	t.Run("file created with 0600 permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		w := envgen.AtomicWriter{}
		if err := w.WriteEnv(path, []byte("KEY=val\n")); err != nil {
			t.Fatalf("WriteEnv() error: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}

		// Mask to permission bits only
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("permissions = %04o, want 0600", perm)
		}
	})

	t.Run("overwrites existing file atomically", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		// Write initial content
		if err := os.WriteFile(path, []byte("OLD_CONTENT=old\n"), 0o600); err != nil {
			t.Fatalf("setup WriteFile: %v", err)
		}

		newData := []byte("NEW_CONTENT=new\n")
		w := envgen.AtomicWriter{}
		if err := w.WriteEnv(path, newData); err != nil {
			t.Fatalf("WriteEnv() error: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile after overwrite: %v", err)
		}

		if string(got) != string(newData) {
			t.Errorf("content after overwrite: got %q, want %q", got, newData)
		}
	})

	t.Run("no leftover .tmp file after successful write", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")

		w := envgen.AtomicWriter{}
		if err := w.WriteEnv(path, []byte("KEY=val\n")); err != nil {
			t.Fatalf("WriteEnv() error: %v", err)
		}

		tmpPath := path + ".tmp"
		if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
			t.Errorf(".tmp file still exists after successful write: %v", err)
		}
	})

	t.Run("idempotent when data is identical", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		data := []byte("KEY=val\n")

		w := envgen.AtomicWriter{}

		if err := w.WriteEnv(path, data); err != nil {
			t.Fatalf("first WriteEnv() error: %v", err)
		}
		if err := w.WriteEnv(path, data); err != nil {
			t.Fatalf("second WriteEnv() error: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		if string(got) != string(data) {
			t.Errorf("content after second write: got %q, want %q", got, data)
		}
	})

	t.Run("write to non-existent directory returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonexistent", ".env")

		w := envgen.AtomicWriter{}
		err := w.WriteEnv(path, []byte("KEY=val\n"))
		if err == nil {
			t.Error("expected error writing to non-existent parent directory, got nil")
		}
	})
}

func TestFakeWriter(t *testing.T) {
	t.Run("records written data per path", func(t *testing.T) {
		fw := &envgen.FakeWriter{Written: make(map[string][]byte)}
		data := []byte("KEY=val\n")

		if err := fw.WriteEnv("/tmp/test.env", data); err != nil {
			t.Fatalf("WriteEnv() error: %v", err)
		}

		got, ok := fw.Written["/tmp/test.env"]
		if !ok {
			t.Fatal("path not recorded in FakeWriter.Written")
		}
		if string(got) != string(data) {
			t.Errorf("got %q, want %q", got, data)
		}
	})

	t.Run("returns configured error", func(t *testing.T) {
		fw := &envgen.FakeWriter{Written: make(map[string][]byte), Err: fakeWriteErr}

		err := fw.WriteEnv("/tmp/test.env", []byte("data"))
		if err != fakeWriteErr {
			t.Errorf("got %v, want %v", err, fakeWriteErr)
		}
	})
}

var fakeWriteErr = writerError("fake write error")

type writerError string

func (e writerError) Error() string { return string(e) }
