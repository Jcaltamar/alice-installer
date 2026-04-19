package tui

import (
	"os"
	"os/exec"
	"os/user"
)

// envDetector holds injectable function dependencies for environment detection.
// The production path uses real stdlib; tests inject fakes.
//
// Note: BootstrapEnv and DetectEnv are now re-exported from bootstrap.go,
// which delegates to internal/bootstrap. This type stays here exclusively so
// that bootstrap_env_test.go can construct test doubles without importing
// the bootstrap package directly.
type envDetector struct {
	// LookPathFn checks whether a binary name is available in PATH.
	LookPathFn func(name string) (string, error)

	// StatFn checks whether a filesystem path exists.
	StatFn func(path string) (os.FileInfo, error)

	// CurrentUserFn returns the current OS user's username and group IDs.
	CurrentUserFn func() (username string, gids []string, err error)

	// LookupGroupFn returns the GID string for a group name.
	LookupGroupFn func(name string) (gid string, err error)
}

// productionDetector returns an envDetector wired to real stdlib functions.
func productionDetectorLocal() envDetector {
	return envDetector{
		LookPathFn: exec.LookPath,
		StatFn:     os.Stat,
		CurrentUserFn: func() (string, []string, error) {
			u, err := user.Current()
			if err != nil {
				return "", nil, err
			}
			gids, err := u.GroupIds()
			if err != nil {
				return u.Username, nil, err
			}
			return u.Username, gids, nil
		},
		LookupGroupFn: func(name string) (string, error) {
			g, err := user.LookupGroup(name)
			if err != nil {
				return "", err
			}
			return g.Gid, nil
		},
	}
}

// detect runs all environment detections and returns a BootstrapEnv.
func (d envDetector) detect() BootstrapEnv {
	env := BootstrapEnv{}

	// --- UserName ---
	username, gids, userErr := d.CurrentUserFn()
	if userErr == nil && username != "" {
		env.UserName = username
	} else {
		fallback := os.Getenv("USER")
		if fallback != "" {
			env.UserName = fallback
		} else {
			env.UserName = "$USER"
		}
	}

	// --- DockerBinaryPresent ---
	if _, err := d.LookPathFn("docker"); err == nil {
		env.DockerBinaryPresent = true
	}

	// --- UserInDockerGroup ---
	if userErr == nil && len(gids) > 0 {
		dockerGID, groupErr := d.LookupGroupFn("docker")
		if groupErr == nil && dockerGID != "" {
			for _, gid := range gids {
				if gid == dockerGID {
					env.UserInDockerGroup = true
					break
				}
			}
		}
	}

	// --- SystemdPresent ---
	if _, err := d.LookPathFn("systemctl"); err == nil {
		if _, err := d.StatFn("/run/systemd/system"); err == nil {
			env.SystemdPresent = true
		}
	}

	return env
}
