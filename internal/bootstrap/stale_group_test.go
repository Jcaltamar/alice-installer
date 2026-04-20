package bootstrap

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Phase 1: TestShellQuote
// ---------------------------------------------------------------------------

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain ascii",
			input: "abc",
			want:  "'abc'",
		},
		{
			name:  "with spaces",
			input: "hello world",
			want:  "'hello world'",
		},
		{
			name:  "with single quote",
			input: "it's",
			want:  "'it'\\''s'",
		},
		{
			name:  "double quote passes through",
			input: `a"b`,
			want:  `'a"b'`,
		},
		{
			name:  "backslash passes through",
			input: `a\b`,
			want:  `'a\b'`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "''",
		},
		{
			name:  "double-quote and backslash",
			input: `a"b\c`,
			want:  `'a"b\c'`,
		},
		{
			name:  "already-quoted-looking input",
			input: "'quoted'",
			want:  "''\\''quoted'\\'''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 2: TestDetectStaleDockerGroup
// ---------------------------------------------------------------------------

func TestDetectStaleDockerGroup(t *testing.T) {
	tests := []struct {
		name        string
		groupFile   string // contents of /etc/group
		username    string // current user
		processGIDs []int  // what syscall.Getgroups returns
		wantStale   bool
		wantGID     int
		wantErr     bool
	}{
		{
			name: "stale: user in docker group but GID not in process",
			groupFile: "root:x:0:\n" +
				"docker:x:999:testuser\n" +
				"testuser:x:1000:\n",
			username:    "testuser",
			processGIDs: []int{1000, 4},
			wantStale:   true,
			wantGID:     999,
			wantErr:     false,
		},
		{
			name: "not stale: user in docker group and GID present in process",
			groupFile: "root:x:0:\n" +
				"docker:x:999:testuser\n" +
				"testuser:x:1000:\n",
			username:    "testuser",
			processGIDs: []int{1000, 999, 4},
			wantStale:   false,
			wantGID:     999,
			wantErr:     false,
		},
		{
			name: "not stale: user not in docker group",
			groupFile: "root:x:0:\n" +
				"docker:x:999:otheruser\n" +
				"testuser:x:1000:\n",
			username:    "testuser",
			processGIDs: []int{1000},
			wantStale:   false,
			wantGID:     0,
			wantErr:     false,
		},
		{
			name: "not stale: no docker group in /etc/group",
			groupFile: "root:x:0:\n" +
				"testuser:x:1000:\n",
			username:    "testuser",
			processGIDs: []int{1000},
			wantStale:   false,
			wantGID:     0,
			wantErr:     false,
		},
		{
			name:        "malformed /etc/group — expect stale=false, no error",
			groupFile:   "this is not valid\n:::\ndocker:bad-gid:notanumber:user\n",
			username:    "testuser",
			processGIDs: []int{1000},
			wantStale:   false,
			wantGID:     0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := staleGroupDetector{
				readGroupFn: func() ([]byte, error) {
					return []byte(tt.groupFile), nil
				},
				getgroupsFn: func() ([]int, error) {
					return tt.processGIDs, nil
				},
				currentFn: func() (string, error) {
					return tt.username, nil
				},
			}

			result, err := d.detect()
			if (err != nil) != tt.wantErr {
				t.Fatalf("detect() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if result.Stale != tt.wantStale {
				t.Errorf("Stale = %v, want %v", result.Stale, tt.wantStale)
			}
			if result.DockerGID != tt.wantGID {
				t.Errorf("DockerGID = %v, want %v", result.DockerGID, tt.wantGID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 3: TestReexecWithDockerGroup
// ---------------------------------------------------------------------------

func TestReexecWithDockerGroup(t *testing.T) {
	type execCall struct {
		argv0 string
		argv  []string
		env   []string
	}

	tests := []struct {
		name          string
		argv          []string
		env           []string
		lookPathResult string
		lookPathErr   error
		execErr       error
		wantExecCall  *execCall // nil means exec should NOT be called
		wantErr       bool
	}{
		{
			name:           "sg found and exec succeeds",
			argv:           []string{"/home/testuser/alice-installer", "--unattended", "--workspace-name=e2e"},
			env:            []string{"HOME=/home/testuser", "PATH=/usr/bin"},
			lookPathResult: "/usr/bin/sg",
			lookPathErr:    nil,
			execErr:        nil,
			wantExecCall: &execCall{
				argv0: "/usr/bin/sg",
				argv:  []string{"sg", "docker", "-c", "'/home/testuser/alice-installer' '--unattended' '--workspace-name=e2e'"},
				env:   []string{"HOME=/home/testuser", "PATH=/usr/bin"},
			},
			wantErr: false,
		},
		{
			name:           "sg not found — LookPath error",
			argv:           []string{"/usr/local/bin/alice-installer"},
			env:            []string{},
			lookPathResult: "",
			lookPathErr:    &lookPathError{name: "sg"},
			execErr:        nil,
			wantExecCall:   nil,
			wantErr:        true,
		},
		{
			name:           "sg found but execFn returns error",
			argv:           []string{"/home/testuser/alice-installer"},
			env:            []string{"PATH=/usr/bin"},
			lookPathResult: "/usr/bin/sg",
			lookPathErr:    nil,
			execErr:        errExecFailed,
			wantExecCall: &execCall{
				argv0: "/usr/bin/sg",
				argv:  []string{"sg", "docker", "-c", "'/home/testuser/alice-installer'"},
				env:   []string{"PATH=/usr/bin"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorded *execCall

			d := reexecDispatcher{
				lookPathFn: func(name string) (string, error) {
					return tt.lookPathResult, tt.lookPathErr
				},
				execFn: func(argv0 string, argv []string, envv []string) error {
					recorded = &execCall{argv0: argv0, argv: argv, env: envv}
					return tt.execErr
				},
			}

			err := d.reexec(tt.argv, tt.env)
			if (err != nil) != tt.wantErr {
				t.Fatalf("reexec() error = %v, wantErr = %v", err, tt.wantErr)
			}

			if tt.wantExecCall == nil {
				if recorded != nil {
					t.Errorf("expected execFn NOT to be called, but it was called with argv0=%q", recorded.argv0)
				}
				return
			}

			if recorded == nil {
				t.Fatalf("expected execFn to be called, but it was not")
			}
			if recorded.argv0 != tt.wantExecCall.argv0 {
				t.Errorf("execFn argv0 = %q, want %q", recorded.argv0, tt.wantExecCall.argv0)
			}
			if len(recorded.argv) != len(tt.wantExecCall.argv) {
				t.Errorf("execFn argv len = %d, want %d\ngot:  %v\nwant: %v",
					len(recorded.argv), len(tt.wantExecCall.argv), recorded.argv, tt.wantExecCall.argv)
				return
			}
			for i := range recorded.argv {
				if recorded.argv[i] != tt.wantExecCall.argv[i] {
					t.Errorf("execFn argv[%d] = %q, want %q", i, recorded.argv[i], tt.wantExecCall.argv[i])
				}
			}
			if len(recorded.env) != len(tt.wantExecCall.env) {
				t.Errorf("execFn env len = %d, want %d", len(recorded.env), len(tt.wantExecCall.env))
				return
			}
			for i := range recorded.env {
				if recorded.env[i] != tt.wantExecCall.env[i] {
					t.Errorf("execFn env[%d] = %q, want %q", i, recorded.env[i], tt.wantExecCall.env[i])
				}
			}
		})
	}
}
