package preflight_test

import (
	"testing"

	"github.com/jcaltamar/alice-installer/internal/preflight"
)

// makeResult creates a CheckResult with the given status.
func makeResult(id preflight.CheckID, status preflight.Status) preflight.CheckResult {
	return preflight.CheckResult{
		ID:     id,
		Status: status,
		Title:  string(id),
	}
}

// ---------------------------------------------------------------------------
// HasBlockingFailure
// ---------------------------------------------------------------------------

func TestReport_HasBlockingFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []preflight.CheckResult
		want  bool
	}{
		{
			name:  "empty report — no failure",
			items: nil,
			want:  false,
		},
		{
			name: "all PASS — no failure",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusPass),
				makeResult(preflight.CheckArch, preflight.StatusPass),
				makeResult(preflight.CheckDockerDaemon, preflight.StatusPass),
			},
			want: false,
		},
		{
			name: "one WARN — no failure",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusPass),
				makeResult(preflight.CheckGPU, preflight.StatusWarn),
			},
			want: false,
		},
		{
			name: "only WARNs — no failure",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckGPU, preflight.StatusWarn),
				makeResult(preflight.CheckPortsAvailable, preflight.StatusWarn),
			},
			want: false,
		},
		{
			name: "one FAIL — blocking",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusFail),
			},
			want: true,
		},
		{
			name: "mixed PASS + WARN + FAIL — blocking",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusPass),
				makeResult(preflight.CheckGPU, preflight.StatusWarn),
				makeResult(preflight.CheckDockerDaemon, preflight.StatusFail),
			},
			want: true,
		},
		{
			name: "multiple FAILs — blocking",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusFail),
				makeResult(preflight.CheckArch, preflight.StatusFail),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := preflight.Report{Items: tc.items}
			if got := r.HasBlockingFailure(); got != tc.want {
				t.Errorf("HasBlockingFailure() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CanContinue (inverse of HasBlockingFailure)
// ---------------------------------------------------------------------------

func TestReport_CanContinue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []preflight.CheckResult
		want  bool
	}{
		{
			name:  "empty report — can continue",
			items: nil,
			want:  true,
		},
		{
			name: "all PASS — can continue",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusPass),
				makeResult(preflight.CheckArch, preflight.StatusPass),
			},
			want: true,
		},
		{
			name: "WARN only — can continue",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckGPU, preflight.StatusWarn),
			},
			want: true,
		},
		{
			name: "any FAIL — cannot continue",
			items: []preflight.CheckResult{
				makeResult(preflight.CheckOS, preflight.StatusPass),
				makeResult(preflight.CheckDockerDaemon, preflight.StatusFail),
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := preflight.Report{Items: tc.items}
			if got := r.CanContinue(); got != tc.want {
				t.Errorf("CanContinue() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Warnings / Failures / Passes — filter methods
// ---------------------------------------------------------------------------

func TestReport_Warnings(t *testing.T) {
	t.Parallel()

	items := []preflight.CheckResult{
		makeResult(preflight.CheckOS, preflight.StatusPass),
		makeResult(preflight.CheckGPU, preflight.StatusWarn),
		makeResult(preflight.CheckDockerDaemon, preflight.StatusFail),
		makeResult(preflight.CheckPortsAvailable, preflight.StatusWarn),
	}
	r := preflight.Report{Items: items}

	warns := r.Warnings()
	if len(warns) != 2 {
		t.Fatalf("Warnings() = %d items, want 2", len(warns))
	}
	for _, w := range warns {
		if w.Status != preflight.StatusWarn {
			t.Errorf("Warnings() returned non-WARN item: %v", w.Status)
		}
	}
}

func TestReport_Failures(t *testing.T) {
	t.Parallel()

	items := []preflight.CheckResult{
		makeResult(preflight.CheckOS, preflight.StatusPass),
		makeResult(preflight.CheckGPU, preflight.StatusWarn),
		makeResult(preflight.CheckDockerDaemon, preflight.StatusFail),
		makeResult(preflight.CheckComposeVersion, preflight.StatusFail),
	}
	r := preflight.Report{Items: items}

	fails := r.Failures()
	if len(fails) != 2 {
		t.Fatalf("Failures() = %d items, want 2", len(fails))
	}
	for _, f := range fails {
		if f.Status != preflight.StatusFail {
			t.Errorf("Failures() returned non-FAIL item: %v", f.Status)
		}
	}
}

func TestReport_Passes(t *testing.T) {
	t.Parallel()

	items := []preflight.CheckResult{
		makeResult(preflight.CheckOS, preflight.StatusPass),
		makeResult(preflight.CheckArch, preflight.StatusPass),
		makeResult(preflight.CheckGPU, preflight.StatusWarn),
		makeResult(preflight.CheckDockerDaemon, preflight.StatusFail),
	}
	r := preflight.Report{Items: items}

	passes := r.Passes()
	if len(passes) != 2 {
		t.Fatalf("Passes() = %d items, want 2", len(passes))
	}
	for _, p := range passes {
		if p.Status != preflight.StatusPass {
			t.Errorf("Passes() returned non-PASS item: %v", p.Status)
		}
	}
}

func TestReport_FilterMethods_EmptyReport(t *testing.T) {
	t.Parallel()

	r := preflight.Report{}
	if len(r.Warnings()) != 0 {
		t.Errorf("Warnings() on empty report = %d, want 0", len(r.Warnings()))
	}
	if len(r.Failures()) != 0 {
		t.Errorf("Failures() on empty report = %d, want 0", len(r.Failures()))
	}
	if len(r.Passes()) != 0 {
		t.Errorf("Passes() on empty report = %d, want 0", len(r.Passes()))
	}
}
