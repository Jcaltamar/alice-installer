// Package bootstrap provides shared types and the ClassifyBlockers function used
// by both the TUI and headless installation paths to decide which elevated
// actions are needed to fix a failing preflight report.
package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// ---------------------------------------------------------------------------
// Action ID constants
// ---------------------------------------------------------------------------

const (
	ActionIDDockerInstall = "docker_install"
	ActionIDSystemdStart  = "systemd_start_docker"
	ActionIDDockerGroup   = "docker_group_add"
)

// ---------------------------------------------------------------------------
// Action
// ---------------------------------------------------------------------------

// Action describes a single elevated command the bootstrap phase will execute.
type Action struct {
	ID               string   // stable ID; matches CheckID when remediating a check
	Description      string   // human-readable summary
	Command          string   // binary to run, e.g. "sudo"
	Args             []string // arguments passed to Command
	PostActionBanner string   // optional; non-empty → show interstitial banner after bootstrap completes
}

// ---------------------------------------------------------------------------
// BootstrapEnv
// ---------------------------------------------------------------------------

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
	LookPathFn    func(name string) (string, error)
	StatFn        func(path string) (os.FileInfo, error)
	CurrentUserFn func() (username string, gids []string, err error)
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

	if _, err := d.LookPathFn("docker"); err == nil {
		env.DockerBinaryPresent = true
	}

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

// ---------------------------------------------------------------------------
// Action constructors
// ---------------------------------------------------------------------------

// DockerInstallAction returns the Action that installs Docker via get.docker.com.
func DockerInstallAction() Action {
	return Action{
		ID:          ActionIDDockerInstall,
		Description: "Install Docker engine via get.docker.com",
		Command:     "sudo",
		Args:        []string{"sh", "-c", "curl -fsSL https://get.docker.com | sh"},
	}
}

// SystemdStartDockerAction returns the Action that enables and starts the Docker daemon via systemd.
func SystemdStartDockerAction() Action {
	return Action{
		ID:          ActionIDSystemdStart,
		Description: "Enable and start Docker daemon (systemctl enable --now docker)",
		Command:     "sudo",
		Args:        []string{"systemctl", "enable", "--now", "docker"},
	}
}

// DockerGroupAddAction returns the Action that adds username to the docker group.
func DockerGroupAddAction(username string) Action {
	return Action{
		ID:               ActionIDDockerGroup,
		Description:      fmt.Sprintf("Add %s to the 'docker' group", username),
		Command:          "sudo",
		Args:             []string{"usermod", "-aG", "docker", username},
		PostActionBanner: "Log out and back in (or run `newgrp docker`) for the new group membership to take effect.",
	}
}

// BuildDirAction constructs the Action that creates dir and grants ownership under sudo.
func BuildDirAction(id, dir, username string) Action {
	script := fmt.Sprintf("mkdir -p %s && chown -R %s:%s %s", dir, username, username, dir)
	return Action{
		ID:          id,
		Description: fmt.Sprintf("Create %s and grant ownership to %s", dir, username),
		Command:     "sudo",
		Args:        []string{"sh", "-c", script},
	}
}

// BuildUserDirAction constructs the Action that creates dir WITHOUT sudo.
func BuildUserDirAction(id, dir string) Action {
	return Action{
		ID:          id,
		Description: fmt.Sprintf("Create %s", dir),
		Command:     "mkdir",
		Args:        []string{"-p", dir},
	}
}

// ---------------------------------------------------------------------------
// ClassifyBlockers
// ---------------------------------------------------------------------------

// ClassifyBlockers splits failing items in report into fixable and non-fixable sets.
// env provides host environment information used to decide which Docker actions to offer.
// workspaceDir is the user-editable artifacts directory (~/.config/alice-guardian);
// it does NOT require sudo because ~/.config is owned by the user.
// Actions are returned in priority order:
//
//  1. docker_install (if Docker binary is missing)
//  2. dir-creation actions (media, config, workspace)
//  3. systemd_start_docker (if Docker present, user in group, systemd available)
//  4. docker_group_add (if Docker present but user not in docker group)
func ClassifyBlockers(report preflight.Report, env BootstrapEnv, mediaDir, configDir, workspaceDir string) (fixable []Action, nonFixable []preflight.CheckResult) {
	username := env.UserName
	if username == "" {
		username = "$USER"
	}

	var dockerInstall []Action
	var dirActions []Action
	var systemdActions []Action
	var groupActions []Action

	for _, item := range report.Items {
		if item.Status != preflight.StatusFail {
			continue
		}
		switch item.ID {
		case preflight.CheckDockerDaemon:
			switch {
			case !env.DockerBinaryPresent:
				dockerInstall = append(dockerInstall, DockerInstallAction())
			case !env.UserInDockerGroup:
				groupActions = append(groupActions, DockerGroupAddAction(username))
			case env.SystemdPresent:
				systemdActions = append(systemdActions, SystemdStartDockerAction())
			default:
				nonFixable = append(nonFixable, item)
			}
		case preflight.CheckDockerVersion, preflight.CheckComposeVersion:
			if env.DockerBinaryPresent {
				nonFixable = append(nonFixable, item)
			}
		case preflight.CheckMediaWritable:
			dirActions = append(dirActions, BuildDirAction(string(preflight.CheckMediaWritable), mediaDir, username))
		case preflight.CheckConfigWritable:
			dirActions = append(dirActions, BuildDirAction(string(preflight.CheckConfigWritable), configDir, username))
		case preflight.CheckWorkspaceWritable:
			dirActions = append(dirActions, BuildUserDirAction(string(preflight.CheckWorkspaceWritable), workspaceDir))
		default:
			nonFixable = append(nonFixable, item)
		}
	}

	fixable = append(fixable, dockerInstall...)
	fixable = append(fixable, dirActions...)
	fixable = append(fixable, systemdActions...)
	fixable = append(fixable, groupActions...)

	return fixable, nonFixable
}
