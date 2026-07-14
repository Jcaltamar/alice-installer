package installation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxProcIdentity(t *testing.T) {
	t.Run("reads canonical cwd executable and start ticks", func(t *testing.T) {
		procRoot := t.TempDir()
		cwd := t.TempDir()
		executable := t.TempDir()
		writeProcFixture(t, procRoot, 42, cwd, executable, procStat(12345))

		identity, err := (LinuxProcIdentity{ProcRoot: procRoot}).Read(context.Background(), 42)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if identity != (ProcIdentity{CWD: cwd, ExecPath: executable, StartTicks: 12345}) {
			t.Fatalf("Read() = %#v, want canonical identity", identity)
		}
	})

	t.Run("rejects missing permission denied oversized zero and invalid evidence", func(t *testing.T) {
		procRoot := t.TempDir()
		writeProcFixture(t, procRoot, 7, t.TempDir(), t.TempDir(), procStat(1))

		tests := []struct {
			name   string
			reader LinuxProcIdentity
		}{
			{"missing", LinuxProcIdentity{ProcRoot: filepath.Join(procRoot, "missing")}},
			{"permission denied", LinuxProcIdentity{ProcRoot: procRoot, ReadLink: func(string) (string, error) { return "", fs.ErrPermission }}},
			{"oversized stat", LinuxProcIdentity{ProcRoot: procRoot, MaxStatBytes: 8}},
			{"zero start ticks", procReaderFor(t, procRoot, procStat(0))},
			{"invalid start ticks", procReaderFor(t, procRoot, []byte("7 (worker) S invalid"))},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := tt.reader.Read(context.Background(), 7); err == nil {
					t.Fatal("Read() error = nil, want fail-closed error")
				}
			})
		}
	})

	t.Run("honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := (LinuxProcIdentity{ProcRoot: t.TempDir()}).Read(ctx, 1); !errors.Is(err, context.Canceled) {
			t.Fatalf("Read() error = %v, want context.Canceled", err)
		}
	})
}

func TestParseProcStartTicks(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint64
	}{
		{"command contains spaces", procStat(99), 99},
		{"command contains closing parenthesis", []byte("8 (worker) name) " + strings.TrimPrefix(string(procStat(99)), "8 (worker) ")), 99},
		{"invalid process state", []byte(strings.Replace(string(procStat(99)), " S ", " invalid ", 1)), 0},
		{"zero is invalid", procStat(0), 0},
		{"truncated is invalid", []byte("8 (worker) S 1"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProcStartTicks(tt.data)
			if tt.want == 0 {
				if err == nil {
					t.Fatal("ParseProcStartTicks() error = nil, want error")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseProcStartTicks() = %d, %v; want %d, nil", got, err, tt.want)
			}
		})
	}
}

func writeProcFixture(t *testing.T, procRoot string, pid int, cwd, executable string, stat []byte) {
	t.Helper()
	dir := filepath.Join(procRoot, fmt.Sprint(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"cwd": cwd, "exe": executable} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), stat, 0o600); err != nil {
		t.Fatal(err)
	}
}

func procReaderFor(t *testing.T, procRoot string, stat []byte) LinuxProcIdentity {
	t.Helper()
	if err := os.WriteFile(filepath.Join(procRoot, "7", "stat"), stat, 0o600); err != nil {
		t.Fatal(err)
	}
	return LinuxProcIdentity{ProcRoot: procRoot}
}

func procStat(ticks uint64) []byte {
	fields := make([]string, 20)
	fields[0] = "S"
	for i := 1; i < len(fields); i++ {
		fields[i] = "1"
	}
	fields[19] = fmt.Sprint(ticks)
	return []byte("7 (worker process) " + strings.Join(fields, " "))
}
