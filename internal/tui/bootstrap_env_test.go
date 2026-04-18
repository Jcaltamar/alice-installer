package tui

import (
	"errors"
	"os"
	"testing"
)

// fakeUserInfo is a minimal stand-in for os/user.User.
type fakeUserInfo struct {
	username string
	gids     []string
}

// fakeGroupInfo is a minimal stand-in for os/user.Group.
type fakeGroupInfo struct {
	gid string
}

// ---------------------------------------------------------------------------
// Tests for envDetector.detect()
// ---------------------------------------------------------------------------

func makeDetector(
	lookPathFn func(string) (string, error),
	statFn func(string) (os.FileInfo, error),
	currentUserFn func() (string, []string, error), // returns username, gids, err
	lookupGroupFn func(string) (string, error), // returns gid, err
) envDetector {
	return envDetector{
		LookPathFn:    lookPathFn,
		StatFn:        statFn,
		CurrentUserFn: currentUserFn,
		LookupGroupFn: lookupGroupFn,
	}
}

func lookPathAlwaysFound(_ string) (string, error)    { return "/usr/bin/x", nil }
func lookPathAlwaysMissing(_ string) (string, error)  { return "", errors.New("not found") }
func statAlwaysExists(_ string) (os.FileInfo, error)  { return nil, nil }
func statAlwaysMissing(_ string) (os.FileInfo, error) { return nil, errors.New("no such file") }

func TestDetectEnvDockerBinaryPresent(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysFound,
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{"1000"}, nil },
		func(_ string) (string, error) { return "999", nil }, // docker group gid != user gid
	)
	env := d.detect()
	if !env.DockerBinaryPresent {
		t.Error("DockerBinaryPresent should be true when docker is found in PATH")
	}
}

func TestDetectEnvDockerBinaryMissing(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{}, nil },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.DockerBinaryPresent {
		t.Error("DockerBinaryPresent should be false when docker is not found in PATH")
	}
}

func TestDetectEnvUserInDockerGroupTrue(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{"1000", "999", "1001"}, nil },
		func(_ string) (string, error) { return "999", nil }, // docker group GID = "999"
	)
	env := d.detect()
	if !env.UserInDockerGroup {
		t.Error("UserInDockerGroup should be true when user's GIDs include docker group GID")
	}
}

func TestDetectEnvUserNotInDockerGroup(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{"1000", "1001"}, nil },
		func(_ string) (string, error) { return "999", nil }, // not in user's GIDs
	)
	env := d.detect()
	if env.UserInDockerGroup {
		t.Error("UserInDockerGroup should be false when docker GID is not in user GIDs")
	}
}

func TestDetectEnvUserInDockerGroupErrorFalse(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "", nil, errors.New("user lookup failed") },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.UserInDockerGroup {
		t.Error("UserInDockerGroup should be false on user lookup error")
	}
}

func TestDetectEnvDockerGroupLookupErrorFalse(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{"1000", "999"}, nil },
		func(_ string) (string, error) { return "", errors.New("group not found") },
	)
	env := d.detect()
	if env.UserInDockerGroup {
		t.Error("UserInDockerGroup should be false when docker group lookup fails")
	}
}

func TestDetectEnvSystemdPresent(t *testing.T) {
	callCount := 0
	d := makeDetector(
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/usr/bin/systemctl", nil
			}
			return "", errors.New("not found")
		},
		func(path string) (os.FileInfo, error) {
			callCount++
			if path == "/run/systemd/system" {
				return nil, nil // exists
			}
			return nil, errors.New("missing")
		},
		func() (string, []string, error) { return "alice", []string{}, nil },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if !env.SystemdPresent {
		t.Error("SystemdPresent should be true when systemctl found AND /run/systemd/system exists")
	}
}

func TestDetectEnvSystemdMissingWhenSystemctlNotFound(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysExists,
		func() (string, []string, error) { return "alice", []string{}, nil },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.SystemdPresent {
		t.Error("SystemdPresent should be false when systemctl is not found")
	}
}

func TestDetectEnvSystemdMissingWhenRunSystemdDirMissing(t *testing.T) {
	d := makeDetector(
		func(name string) (string, error) {
			if name == "systemctl" {
				return "/usr/bin/systemctl", nil
			}
			return "", errors.New("not found")
		},
		statAlwaysMissing,
		func() (string, []string, error) { return "alice", []string{}, nil },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.SystemdPresent {
		t.Error("SystemdPresent should be false when /run/systemd/system does not exist")
	}
}

func TestDetectEnvUserNameFromCurrentUser(t *testing.T) {
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "bob", []string{}, nil },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.UserName != "bob" {
		t.Errorf("UserName = %q, want bob", env.UserName)
	}
}

func TestDetectEnvUserNameFallbackToEnvVar(t *testing.T) {
	t.Setenv("USER", "fallback_user")
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "", nil, errors.New("no user") },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.UserName != "fallback_user" {
		t.Errorf("UserName = %q, want fallback_user", env.UserName)
	}
}

func TestDetectEnvUserNameFallbackLiteral(t *testing.T) {
	t.Setenv("USER", "")
	d := makeDetector(
		lookPathAlwaysMissing,
		statAlwaysMissing,
		func() (string, []string, error) { return "", nil, errors.New("no user") },
		func(_ string) (string, error) { return "999", nil },
	)
	env := d.detect()
	if env.UserName != "$USER" {
		t.Errorf("UserName = %q, want $USER literal fallback", env.UserName)
	}
}

// TestDetectEnvTableDriven covers important permutations of the four boolean fields.
func TestDetectEnvTableDriven(t *testing.T) {
	tests := []struct {
		name                string
		dockerFound         bool
		systemctlFound      bool
		systemdDirExists    bool
		userGIDs            []string
		dockerGroupGID      string
		groupLookupErr      bool
		wantDockerPresent   bool
		wantInGroup         bool
		wantSystemd         bool
	}{
		{
			name: "all present, in group",
			dockerFound: true, systemctlFound: true, systemdDirExists: true,
			userGIDs: []string{"1000", "999"}, dockerGroupGID: "999",
			wantDockerPresent: true, wantInGroup: true, wantSystemd: true,
		},
		{
			name: "docker present, not in group, no systemd",
			dockerFound: true, systemctlFound: false, systemdDirExists: false,
			userGIDs: []string{"1000"}, dockerGroupGID: "999",
			wantDockerPresent: true, wantInGroup: false, wantSystemd: false,
		},
		{
			name: "nothing present",
			dockerFound: false, systemctlFound: false, systemdDirExists: false,
			userGIDs: []string{}, dockerGroupGID: "",
			groupLookupErr:    true,
			wantDockerPresent: false, wantInGroup: false, wantSystemd: false,
		},
		{
			name: "docker present, in group, systemctl found but no /run/systemd/system",
			dockerFound: true, systemctlFound: true, systemdDirExists: false,
			userGIDs: []string{"999"}, dockerGroupGID: "999",
			wantDockerPresent: true, wantInGroup: true, wantSystemd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPath := func(name string) (string, error) {
				switch name {
				case "docker":
					if tt.dockerFound {
						return "/usr/bin/docker", nil
					}
					return "", errors.New("not found")
				case "systemctl":
					if tt.systemctlFound {
						return "/usr/bin/systemctl", nil
					}
					return "", errors.New("not found")
				}
				return "", errors.New("not found")
			}
			statFn := func(path string) (os.FileInfo, error) {
				if path == "/run/systemd/system" && tt.systemdDirExists {
					return nil, nil
				}
				return nil, errors.New("missing")
			}
			currentUserFn := func() (string, []string, error) {
				return "testuser", tt.userGIDs, nil
			}
			lookupGroupFn := func(name string) (string, error) {
				if tt.groupLookupErr {
					return "", errors.New("group not found")
				}
				return tt.dockerGroupGID, nil
			}

			d := makeDetector(lookPath, statFn, currentUserFn, lookupGroupFn)
			env := d.detect()

			if env.DockerBinaryPresent != tt.wantDockerPresent {
				t.Errorf("DockerBinaryPresent = %v, want %v", env.DockerBinaryPresent, tt.wantDockerPresent)
			}
			if env.UserInDockerGroup != tt.wantInGroup {
				t.Errorf("UserInDockerGroup = %v, want %v", env.UserInDockerGroup, tt.wantInGroup)
			}
			if env.SystemdPresent != tt.wantSystemd {
				t.Errorf("SystemdPresent = %v, want %v", env.SystemdPresent, tt.wantSystemd)
			}
		})
	}
}
