package tui

import (
	"os"
	"os/exec"
	"os/user"
)

// BootstrapEnv captures the host environment state relevant to Docker bootstrap decisions.
// All fields are computed once at startup via DetectEnv().
type BootstrapEnv struct {
	UserName            string // current OS username; fallback $USER; fallback "$USER" literal
	DockerBinaryPresent bool   // true when exec.LookPath("docker") succeeds
	UserInDockerGroup   bool   // true when current user's GIDs include the "docker" group GID
	SystemdPresent      bool   // true when systemctl is in PATH AND /run/systemd/system exists
}

// envDetector holds injectable function dependencies for environment detection.
// The production path uses real stdlib; tests inject fakes.
type envDetector struct {
	// LookPathFn checks whether a binary name is available in PATH.
	LookPathFn func(name string) (string, error)

	// StatFn checks whether a filesystem path exists.
	StatFn func(path string) (os.FileInfo, error)

	// CurrentUserFn returns the current OS user's username and group IDs.
	// Returns (username string, gids []string, err error).
	CurrentUserFn func() (username string, gids []string, err error)

	// LookupGroupFn returns the GID string for a group name.
	LookupGroupFn func(name string) (gid string, err error)
}

// productionDetector returns an envDetector wired to real stdlib functions.
func productionDetector() envDetector {
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
	// Only attempt if we have valid user info (no error and gids available).
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

// DetectEnv returns a BootstrapEnv populated from the real host environment.
// Safe to call at startup; never panics (all failures result in conservative false values).
func DetectEnv() BootstrapEnv {
	return productionDetector().detect()
}
