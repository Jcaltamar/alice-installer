package runlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	SchemaVersion = 1
	DefaultKeep   = 20
)

type Fields struct {
	Mode                string   `json:"mode,omitempty"`
	Version             string   `json:"version,omitempty"`
	Debug               *bool    `json:"debug,omitempty"`
	Unattended          *bool    `json:"unattended,omitempty"`
	DryRun              *bool    `json:"dry_run,omitempty"`
	Disposition         string   `json:"disposition,omitempty"`
	ErrorClass          string   `json:"error_class,omitempty"`
	Operation           string   `json:"operation,omitempty"`
	Cause               string   `json:"cause,omitempty"`
	Targets             []Target `json:"targets,omitempty"`
	Attempted           int      `json:"attempted,omitempty"`
	Recovered           int      `json:"recovered,omitempty"`
	Verified            *bool    `json:"verified,omitempty"`
	Mutated             *bool    `json:"mutated,omitempty"`
	BackendHealthy      *bool    `json:"backend_healthy,omitempty"`
	Rollback            string   `json:"rollback,omitempty"`
	LegacyValidated     *bool    `json:"legacy_validated,omitempty"`
	TargetValidated     *bool    `json:"target_validated,omitempty"`
	RestoreExitOK       *bool    `json:"restore_exit_ok,omitempty"`
	ConnectionOK        *bool    `json:"connection_ok,omitempty"`
	PostgreSQLReachable *bool    `json:"postgresql_reachable,omitempty"`
}

type Target struct {
	PMID int64  `json:"pmid"`
	Name string `json:"name"`
	Port uint16 `json:"port"`
}

type Event struct {
	Event  string
	Stage  string
	Status string
	Code   string
	Fields Fields
}

type record struct {
	SchemaVersion int    `json:"schema_version"`
	Timestamp     string `json:"timestamp"`
	RunID         string `json:"run_id"`
	Sequence      uint64 `json:"sequence"`
	Event         string `json:"event"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	Code          string `json:"code,omitempty"`
	Fields        Fields `json:"fields,omitempty"`
}

type Logger interface {
	Log(Event)
	Path() string
	Warning() string
	Close() error
}

type JSONL struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	runID   string
	seq     uint64
	warning string
	now     func() time.Time
}

func Open(dir string, keep int) (*JSONL, error) {
	if dir == "" {
		state, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(state, ".local", "state", "alice-installer", "logs")
	}
	if err := secureDir(dir); err != nil {
		return nil, err
	}
	idBytes := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, idBytes); err != nil {
		return nil, err
	}
	runID := hex.EncodeToString(idBytes)
	name := fmt.Sprintf("alice-installer-%s-%s.jsonl", time.Now().UTC().Format("20060102T150405.000000000Z"), runID)
	path := filepath.Join(dir, name)
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	logger := &JSONL{file: file, path: path, runID: runID, now: time.Now}
	if keep <= 0 {
		keep = DefaultKeep
	}
	if err := retain(dir, keep, filepath.Base(path)); err != nil {
		logger.warning = "old diagnostic logs could not be pruned"
	}
	return logger, nil
}

func secureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("diagnostic log directory is not a private directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return nil
}

func (l *JSONL) Log(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil || l.warning != "" && strings.HasPrefix(l.warning, "diagnostic logging stopped") {
		return
	}
	l.seq++
	r := record{SchemaVersion: SchemaVersion, Timestamp: l.now().UTC().Format(time.RFC3339Nano), RunID: l.runID, Sequence: l.seq,
		Event: safe(event.Event), Stage: safe(event.Stage), Status: safe(event.Status), Code: safe(event.Code), Fields: sanitize(event.Fields)}
	data, err := json.Marshal(r)
	if err == nil {
		_, err = l.file.Write(append(data, '\n'))
	}
	if err != nil {
		l.warning = "diagnostic logging stopped after a write failure"
	}
}

func (l *JSONL) Path() string { return l.path }
func (l *JSONL) Warning() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.warning
}
func (l *JSONL) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Sync()
	err = errors.Join(err, l.file.Close())
	l.file = nil
	return err
}

func sanitize(f Fields) Fields {
	f.Mode, f.Version, f.Disposition = safe(f.Mode), safe(f.Version), safe(f.Disposition)
	f.ErrorClass, f.Operation, f.Cause, f.Rollback = safe(f.ErrorClass), safe(f.Operation), safe(f.Cause), safe(f.Rollback)
	if len(f.Targets) > 16 {
		f.Targets = f.Targets[:16]
	}
	for i := range f.Targets {
		f.Targets[i].Name = safe(f.Targets[i].Name)
	}
	return f
}

func safe(value string) string {
	if len(value) > 96 {
		value = value[:96]
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "secret", "token", "credential", "database_url", "postgres://", "postgresql://", "pgpass", "ami_"} {
		if strings.Contains(lower, marker) {
			return "redacted"
		}
	}
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return "redacted"
		}
	}
	return value
}

func retain(dir string, keep int, current string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, "alice-installer-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		names = append(names, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) <= keep {
		return nil
	}
	for _, name := range names[keep:] {
		if name == current {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

type Disabled struct{ warning string }

func NewDisabled() *Disabled        { return &Disabled{warning: "diagnostic log unavailable"} }
func (*Disabled) Log(Event)         {}
func (*Disabled) Path() string      { return "unavailable" }
func (d *Disabled) Warning() string { return d.warning }
func (*Disabled) Close() error      { return nil }
