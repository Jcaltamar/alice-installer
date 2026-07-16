package runlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenWritesPrivateSequentialJSONL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l, err := Open(dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	l.Log(Event{Event: "startup", Stage: "run", Status: "started"})
	l.Log(Event{Event: "terminal", Stage: "run", Status: "success"})
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, l.Path(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
	f, err := os.Open(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	sequence := uint64(1)
	for s.Scan() {
		var got record
		if err := json.Unmarshal(s.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Sequence != sequence || got.SchemaVersion != SchemaVersion {
			t.Fatalf("record = %#v", got)
		}
		sequence++
	}
	if sequence != 3 || s.Err() != nil {
		t.Fatalf("sequence=%d err=%v", sequence, s.Err())
	}
}

func TestOpenRejectsSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "logs")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, 20); err == nil {
		t.Fatal("Open accepted symlink directory")
	}
}

func TestRetentionLeavesNonmatchingFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := os.WriteFile(filepath.Join(dir, "alice-installer-20260101T00000"+string(rune('0'+i))+"Z-id.jsonl"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(dir, "keep-me.txt")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("entries=%d, want 3", len(entries))
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("nonmatching file removed")
	}
}

func TestEveryStringFieldRedactsCanarySecrets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l, err := Open(dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	canary := "postgres://canary-secret"
	l.Log(Event{Event: canary, Stage: canary, Status: canary, Code: canary, Fields: Fields{Mode: canary, Version: canary, Disposition: canary, ErrorClass: canary, Operation: canary, Cause: canary, Rollback: canary, Targets: []Target{{Name: canary}}}})
	_ = l.Close()
	data, err := os.ReadFile(l.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), canary) || strings.Contains(string(data), "canary-secret") {
		t.Fatalf("secret leaked: %s", data)
	}
}

func TestWriteFailureDegradesToWarning(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "logs"), 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.file.Close(); err != nil {
		t.Fatal(err)
	}
	l.Log(Event{Event: "terminal", Stage: "run", Status: "failure"})
	if !strings.Contains(l.Warning(), "logging stopped") {
		t.Fatalf("warning = %q", l.Warning())
	}
}
