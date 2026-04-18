package tui

import (
	"strings"
	"testing"
)

// TestDockerInstallActionConstructor verifies the dockerInstallAction() helper.
func TestDockerInstallActionConstructor(t *testing.T) {
	a := dockerInstallAction()
	if a.ID != ActionIDDockerInstall {
		t.Errorf("ID = %q, want %q", a.ID, ActionIDDockerInstall)
	}
	if a.Command != "sudo" {
		t.Errorf("Command = %q, want sudo", a.Command)
	}
	if len(a.Args) == 0 {
		t.Fatal("Args should not be empty")
	}
	// Should contain get.docker.com
	joined := strings.Join(a.Args, " ")
	if !strings.Contains(joined, "get.docker.com") {
		t.Errorf("Args should reference get.docker.com, got: %s", joined)
	}
	if a.Description == "" {
		t.Error("Description should not be empty")
	}
}

// TestSystemdStartDockerActionConstructor verifies systemdStartDockerAction().
func TestSystemdStartDockerActionConstructor(t *testing.T) {
	a := systemdStartDockerAction()
	if a.ID != ActionIDSystemdStart {
		t.Errorf("ID = %q, want %q", a.ID, ActionIDSystemdStart)
	}
	if a.Command != "sudo" {
		t.Errorf("Command = %q, want sudo", a.Command)
	}
	// Args should be: systemctl enable --now docker
	if len(a.Args) < 3 {
		t.Fatalf("Args too short: %v", a.Args)
	}
	if a.Args[0] != "systemctl" {
		t.Errorf("Args[0] = %q, want systemctl", a.Args[0])
	}
	if a.Description == "" {
		t.Error("Description should not be empty")
	}
}

// TestDockerGroupAddActionConstructor verifies dockerGroupAddAction(username).
func TestDockerGroupAddActionConstructor(t *testing.T) {
	a := dockerGroupAddAction("alice")
	if a.ID != ActionIDDockerGroup {
		t.Errorf("ID = %q, want %q", a.ID, ActionIDDockerGroup)
	}
	if a.Command != "sudo" {
		t.Errorf("Command = %q, want sudo", a.Command)
	}
	// Args should be: usermod -aG docker alice
	if len(a.Args) < 4 {
		t.Fatalf("Args too short: %v", a.Args)
	}
	if a.Args[0] != "usermod" {
		t.Errorf("Args[0] = %q, want usermod", a.Args[0])
	}
	// Last arg should be the username
	if a.Args[len(a.Args)-1] != "alice" {
		t.Errorf("Args last = %q, want alice", a.Args[len(a.Args)-1])
	}
	if a.PostActionBanner == "" {
		t.Error("dockerGroupAddAction must have a non-empty PostActionBanner")
	}
	// Banner must mention log out/in or newgrp
	if !strings.Contains(strings.ToLower(a.PostActionBanner), "log out") &&
		!strings.Contains(a.PostActionBanner, "newgrp") {
		t.Errorf("PostActionBanner should mention 'log out' or 'newgrp', got: %q", a.PostActionBanner)
	}
	if !strings.Contains(a.Description, "alice") {
		t.Errorf("Description should mention username, got: %q", a.Description)
	}
}

// TestActionIDConstants ensures the constants are defined and non-empty.
func TestActionIDConstants(t *testing.T) {
	if ActionIDDockerInstall == "" {
		t.Error("ActionIDDockerInstall should be non-empty")
	}
	if ActionIDSystemdStart == "" {
		t.Error("ActionIDSystemdStart should be non-empty")
	}
	if ActionIDDockerGroup == "" {
		t.Error("ActionIDDockerGroup should be non-empty")
	}
}
