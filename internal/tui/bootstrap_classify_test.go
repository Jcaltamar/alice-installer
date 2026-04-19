package tui

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// healthyEnv returns a BootstrapEnv where Docker is fully working — used in tests
// that don't care about Docker-specific classification behaviour.
func healthyEnv() BootstrapEnv {
	return BootstrapEnv{
		UserName:            "testuser",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      true,
	}
}

// noDockerEnv returns a BootstrapEnv where Docker binary is absent.
func noDockerEnv() BootstrapEnv {
	return BootstrapEnv{
		UserName:            "testuser",
		DockerBinaryPresent: false,
		UserInDockerGroup:   false,
		SystemdPresent:      false,
	}
}

func TestClassifyBlockers(t *testing.T) {
	const mediaDir = "/opt/alice-media"
	const configDir = "/opt/alice-config"

	tests := []struct {
		name           string
		env            BootstrapEnv
		items          []preflight.CheckResult
		wantFixable    int
		wantNonFixable int
	}{
		{
			name: "both dirs fail → 2 fixable 0 non-fixable",
			env:  healthyEnv(),
			items: []preflight.CheckResult{
				{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
				{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
			},
			wantFixable:    2,
			wantNonFixable: 0,
		},
		{
			name: "docker fail (binary missing) + media fail → 2 fixable 0 non-fixable",
			env:  noDockerEnv(),
			items: []preflight.CheckResult{
				{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
				{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
			},
			wantFixable:    2,
			wantNonFixable: 0,
		},
		{
			name: "docker fail (non-systemd stuck) + media fail → 1 fixable 1 non-fixable",
			env: BootstrapEnv{
				UserName:            "testuser",
				DockerBinaryPresent: true,
				UserInDockerGroup:   true,
				SystemdPresent:      false, // non-fixable stuck
			},
			items: []preflight.CheckResult{
				{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
				{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
			},
			wantFixable:    1,
			wantNonFixable: 1,
		},
		{
			name: "only warnings → 0 fixable 0 non-fixable",
			env:  healthyEnv(),
			items: []preflight.CheckResult{
				{ID: preflight.CheckGPU, Status: preflight.StatusWarn, Title: "GPU"},
				{ID: preflight.CheckDockerVersion, Status: preflight.StatusWarn, Title: "DockerVer"},
			},
			wantFixable:    0,
			wantNonFixable: 0,
		},
		{
			name:           "all pass → 0 0",
			env:            healthyEnv(),
			items:          []preflight.CheckResult{{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"}},
			wantFixable:    0,
			wantNonFixable: 0,
		},
		{
			name: "only config fails → 1 fixable 0 non-fixable",
			env:  healthyEnv(),
			items: []preflight.CheckResult{
				{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
				{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
			},
			wantFixable:    1,
			wantNonFixable: 0,
		},
		{
			name: "ports and compose fail (non-fixable) → 0 fixable 2 non-fixable",
			env:  healthyEnv(),
			items: []preflight.CheckResult{
				{ID: preflight.CheckComposeVersion, Status: preflight.StatusFail, Title: "Compose"},
				{ID: preflight.CheckPortsAvailable, Status: preflight.StatusFail, Title: "Ports"},
			},
			wantFixable:    0,
			wantNonFixable: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := preflight.Report{Items: tt.items}
			fixable, nonFixable := ClassifyBlockers(report, tt.env, mediaDir, configDir, "")

			if len(fixable) != tt.wantFixable {
				t.Errorf("fixable = %d, want %d", len(fixable), tt.wantFixable)
			}
			if len(nonFixable) != tt.wantNonFixable {
				t.Errorf("nonFixable = %d, want %d", len(nonFixable), tt.wantNonFixable)
			}
		})
	}
}

func TestClassifyBlockersActionsHaveCorrectCommand(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
			{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
		},
	}
	fixable, _ := ClassifyBlockers(report, healthyEnv(), "/opt/alice-media", "/opt/alice-config", "")
	for _, a := range fixable {
		if a.Command != "sudo" {
			t.Errorf("action %q Command = %q, want sudo", a.ID, a.Command)
		}
		if len(a.Args) == 0 {
			t.Errorf("action %q has empty Args", a.ID)
		}
		if a.Description == "" {
			t.Errorf("action %q has empty Description", a.ID)
		}
	}
}

func TestClassifyBlockersActionsIDsMatchCheckIDs(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail},
			{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail},
		},
	}
	fixable, _ := ClassifyBlockers(report, healthyEnv(), "/m", "/c", "")
	ids := make(map[string]bool)
	for _, a := range fixable {
		ids[a.ID] = true
	}
	if !ids[string(preflight.CheckMediaWritable)] {
		t.Error("expected action ID matching CheckMediaWritable")
	}
	if !ids[string(preflight.CheckConfigWritable)] {
		t.Error("expected action ID matching CheckConfigWritable")
	}
}

// ---------------------------------------------------------------------------
// T-DB-009/010: Docker-missing case
// ---------------------------------------------------------------------------

func TestClassifyBlockersDockerMissingEmitsDockerInstall(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := noDockerEnv()
	fixable, nonFixable := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 1 {
		t.Fatalf("fixable count = %d, want 1", len(fixable))
	}
	if fixable[0].ID != ActionIDDockerInstall {
		t.Errorf("fixable[0].ID = %q, want %q", fixable[0].ID, ActionIDDockerInstall)
	}
	if len(nonFixable) != 0 {
		t.Errorf("nonFixable count = %d, want 0", len(nonFixable))
	}
}

func TestClassifyBlockersDockerMissingActionIsFirst(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
		},
	}
	env := noDockerEnv()
	fixable, _ := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) == 0 {
		t.Fatal("expected at least 1 fixable action")
	}
	if fixable[0].ID != ActionIDDockerInstall {
		t.Errorf("docker_install should be first fixable action, got %q", fixable[0].ID)
	}
}

func TestClassifyBlockersDockerPresentNoBinaryCheck(t *testing.T) {
	// Docker present + all healthy → CheckDockerDaemon PASS → no docker_install
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusPass, Title: "Docker"},
		},
	}
	fixable, _ := ClassifyBlockers(report, healthyEnv(), "/m", "/c", "")
	for _, a := range fixable {
		if a.ID == ActionIDDockerInstall {
			t.Error("docker_install should NOT be emitted when CheckDockerDaemon passes")
		}
	}
}

// ---------------------------------------------------------------------------
// T-DB-011/012: User-not-in-group case
// ---------------------------------------------------------------------------

func TestClassifyBlockersUserNotInGroupEmitsGroupAdd(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
		SystemdPresent:      true,
	}
	fixable, nonFixable := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 1 {
		t.Fatalf("fixable count = %d, want 1", len(fixable))
	}
	if fixable[0].ID != ActionIDDockerGroup {
		t.Errorf("fixable[0].ID = %q, want %q", fixable[0].ID, ActionIDDockerGroup)
	}
	if len(nonFixable) != 0 {
		t.Errorf("nonFixable count = %d, want 0", len(nonFixable))
	}
}

func TestClassifyBlockersGroupAddHasPostActionBanner(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := BootstrapEnv{
		UserName:            "bob",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
		SystemdPresent:      false,
	}
	fixable, _ := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) == 0 {
		t.Fatal("expected docker_group_add action")
	}
	if fixable[0].PostActionBanner == "" {
		t.Error("docker_group_add action must have a non-empty PostActionBanner")
	}
}

func TestClassifyBlockersGroupAddUsesEnvUsername(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := BootstrapEnv{
		UserName:            "charlie",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
	}
	fixable, _ := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) == 0 {
		t.Fatal("expected at least one fixable action")
	}
	found := false
	for _, a := range fixable {
		if a.ID == ActionIDDockerGroup {
			found = true
			// Username should appear in the Args
			for _, arg := range a.Args {
				if arg == "charlie" {
					return // test passes
				}
			}
			t.Errorf("docker_group_add Args should contain username 'charlie', got: %v", a.Args)
		}
	}
	if !found {
		t.Error("expected docker_group_add action in fixable slice")
	}
}

// ---------------------------------------------------------------------------
// T-DB-013/014: Systemctl case
// ---------------------------------------------------------------------------

func TestClassifyBlockersSystemctlCaseEmitsSystemdStart(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      true,
	}
	fixable, nonFixable := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 1 {
		t.Fatalf("fixable count = %d, want 1", len(fixable))
	}
	if fixable[0].ID != ActionIDSystemdStart {
		t.Errorf("fixable[0].ID = %q, want %q", fixable[0].ID, ActionIDSystemdStart)
	}
	if len(nonFixable) != 0 {
		t.Errorf("nonFixable count = %d, want 0", len(nonFixable))
	}
}

// ---------------------------------------------------------------------------
// T-DB-015: Non-systemd stuck case
// ---------------------------------------------------------------------------

func TestClassifyBlockersNonSystemdStuckIsNonFixable(t *testing.T) {
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
		},
	}
	env := BootstrapEnv{
		UserName:            "alice",
		DockerBinaryPresent: true,
		UserInDockerGroup:   true,
		SystemdPresent:      false, // no systemd
	}
	fixable, nonFixable := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 0 {
		t.Errorf("fixable count = %d, want 0 for non-systemd stuck", len(fixable))
	}
	if len(nonFixable) != 1 {
		t.Errorf("nonFixable count = %d, want 1", len(nonFixable))
	}
	if nonFixable[0].ID != preflight.CheckDockerDaemon {
		t.Errorf("nonFixable[0].ID = %q, want %q", nonFixable[0].ID, preflight.CheckDockerDaemon)
	}
}

// ---------------------------------------------------------------------------
// T-DB-016: Priority ordering
// ---------------------------------------------------------------------------

func TestClassifyBlockersActionOrdering(t *testing.T) {
	// Docker missing + both dirs fail → order: docker_install, media, config
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
			{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
			{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
		},
	}
	env := noDockerEnv()
	fixable, _ := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 3 {
		t.Fatalf("fixable count = %d, want 3", len(fixable))
	}
	if fixable[0].ID != ActionIDDockerInstall {
		t.Errorf("fixable[0].ID = %q, want docker_install", fixable[0].ID)
	}
	if fixable[1].ID != string(preflight.CheckMediaWritable) {
		t.Errorf("fixable[1].ID = %q, want media_writable", fixable[1].ID)
	}
	if fixable[2].ID != string(preflight.CheckConfigWritable) {
		t.Errorf("fixable[2].ID = %q, want config_writable", fixable[2].ID)
	}
}

func TestClassifyBlockersActionOrderingGroupAddLast(t *testing.T) {
	// Config dir fail + docker group fail → dirs first, group_add last
	report := preflight.Report{
		Items: []preflight.CheckResult{
			{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
			{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
		},
	}
	env := BootstrapEnv{
		UserName:            "bob",
		DockerBinaryPresent: true,
		UserInDockerGroup:   false,
	}
	fixable, _ := ClassifyBlockers(report, env, "/m", "/c", "")

	if len(fixable) != 2 {
		t.Fatalf("fixable count = %d, want 2", len(fixable))
	}
	if fixable[0].ID != string(preflight.CheckConfigWritable) {
		t.Errorf("fixable[0].ID = %q, want config_writable (dirs before group)", fixable[0].ID)
	}
	if fixable[1].ID != ActionIDDockerGroup {
		t.Errorf("fixable[1].ID = %q, want docker_group_add (last)", fixable[1].ID)
	}
}
