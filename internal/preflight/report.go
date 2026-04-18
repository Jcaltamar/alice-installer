// Package preflight orchestrates host-environment checks before installation.
package preflight

// Status represents the outcome of a single preflight check.
type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

// CheckID is a stable identifier for each preflight check.
type CheckID string

const (
	CheckOS             CheckID = "os"
	CheckArch           CheckID = "arch"
	CheckDockerDaemon   CheckID = "docker_daemon"
	CheckDockerVersion  CheckID = "docker_version"
	CheckComposeVersion CheckID = "compose_version"
	CheckGPU            CheckID = "gpu"
	CheckMediaWritable  CheckID = "media_writable"
	CheckConfigWritable CheckID = "config_writable"
	CheckPortsAvailable CheckID = "ports_available"
)

// CheckResult holds the outcome of one preflight check.
type CheckResult struct {
	ID          CheckID
	Status      Status
	Title       string // short human-readable name
	Detail      string // diagnostic detail
	Remediation string // how to fix if FAIL/WARN
}

// Report aggregates all preflight check results.
type Report struct {
	Items []CheckResult
}

// HasBlockingFailure returns true when at least one check has StatusFail.
func (r Report) HasBlockingFailure() bool {
	for _, item := range r.Items {
		if item.Status == StatusFail {
			return true
		}
	}
	return false
}

// CanContinue returns true when no check has StatusFail (i.e. the inverse of HasBlockingFailure).
func (r Report) CanContinue() bool {
	return !r.HasBlockingFailure()
}

// Warnings returns only the items with StatusWarn.
func (r Report) Warnings() []CheckResult {
	return filterByStatus(r.Items, StatusWarn)
}

// Failures returns only the items with StatusFail.
func (r Report) Failures() []CheckResult {
	return filterByStatus(r.Items, StatusFail)
}

// Passes returns only the items with StatusPass.
func (r Report) Passes() []CheckResult {
	return filterByStatus(r.Items, StatusPass)
}

// filterByStatus is a pure helper that extracts items matching the given status.
func filterByStatus(items []CheckResult, s Status) []CheckResult {
	result := make([]CheckResult, 0, len(items))
	for _, item := range items {
		if item.Status == s {
			result = append(result, item)
		}
	}
	return result
}
