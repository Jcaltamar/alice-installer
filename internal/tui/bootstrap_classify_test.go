package tui

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

func TestClassifyBlockers(t *testing.T) {
	const mediaDir = "/opt/alice-media"
	const configDir = "/opt/alice-config"

	tests := []struct {
		name            string
		items           []preflight.CheckResult
		wantFixable     int
		wantNonFixable  int
	}{
		{
			name: "both dirs fail → 2 fixable 0 non-fixable",
			items: []preflight.CheckResult{
				{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
				{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
			},
			wantFixable:    2,
			wantNonFixable: 0,
		},
		{
			name: "docker+media fail → 1 fixable 1 non-fixable",
			items: []preflight.CheckResult{
				{ID: preflight.CheckDockerDaemon, Status: preflight.StatusFail, Title: "Docker"},
				{ID: preflight.CheckMediaWritable, Status: preflight.StatusFail, Title: "Media"},
			},
			wantFixable:    1,
			wantNonFixable: 1,
		},
		{
			name: "only warnings → 0 fixable 0 non-fixable",
			items: []preflight.CheckResult{
				{ID: preflight.CheckGPU, Status: preflight.StatusWarn, Title: "GPU"},
				{ID: preflight.CheckDockerVersion, Status: preflight.StatusWarn, Title: "DockerVer"},
			},
			wantFixable:    0,
			wantNonFixable: 0,
		},
		{
			name:           "all pass → 0 0",
			items:          []preflight.CheckResult{{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"}},
			wantFixable:    0,
			wantNonFixable: 0,
		},
		{
			name: "only config fails → 1 fixable 0 non-fixable",
			items: []preflight.CheckResult{
				{ID: preflight.CheckOS, Status: preflight.StatusPass, Title: "OS"},
				{ID: preflight.CheckConfigWritable, Status: preflight.StatusFail, Title: "Config"},
			},
			wantFixable:    1,
			wantNonFixable: 0,
		},
		{
			name: "ports and compose fail (non-fixable) → 0 fixable 2 non-fixable",
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
			fixable, nonFixable := ClassifyBlockers(report, mediaDir, configDir)

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
	fixable, _ := ClassifyBlockers(report, "/opt/alice-media", "/opt/alice-config")
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
	fixable, _ := ClassifyBlockers(report, "/m", "/c")
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
