package installation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const guardianRoot = "/opt/alice-guardian"
const backendRoot = "/opt/backend_alice_guardian"
const defaultPM2AcquisitionTimeout = 5 * time.Second
const defaultPM2AcquisitionOutputLimit = 64 * 1024
const defaultPM2StopProofTimeout = 2 * time.Second
const defaultPM2StopProofInterval = 100 * time.Millisecond

type PM2Record struct {
	ID                          int64
	PID                         int
	Name, CWD, ExecPath, Status string
}
type PM2ProcessIdentity struct {
	PMID            int64
	Name            string
	PID             int
	CWD             string
	ExecPath        string
	RuntimeExecPath string
	Port            uint16
	StartTicks      uint64
}
type PM2StoppedEvidence struct {
	PMID                 int64
	OriginalPID          int
	Port                 uint16
	StartTicks           uint64
	StopCommandSucceeded bool
	StopVerified         bool
}
type PM2Quiescence struct {
	Processes []PM2ProcessIdentity
	Evidence  []PM2StoppedEvidence
}
type PM2Recovery struct {
	Attempted, Recovered int
	Verified             bool
	Code                 string
}

type QuiescenceError struct {
	Code       string
	Diagnostic *PM2ObservationDiagnostic
}

func (e QuiescenceError) Error() string {
	if e.Diagnostic != nil {
		return e.Code + ": " + e.Diagnostic.String()
	}
	return e.Code
}

// LegacyPM2Quiescer lets migration own a lease without PM2 command construction.
type LegacyPM2Quiescer interface {
	Quiesce(context.Context) (PM2Quiescence, error)
	Recover(context.Context, PM2Quiescence) (PM2Recovery, error)
}
type pm2JSONRecord struct {
	ID       int64  `json:"pm_id"`
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	ExecPath string `json:"pm_exec_path"`
	Env      struct {
		CWD    string `json:"cwd"`
		PMCWD  string `json:"pm_cwd"`
		Status string `json:"status"`
		PID    int    `json:"pid"`
		Exec   string `json:"pm_exec_path"`
	} `json:"pm2_env"`
}

// LinuxPM2Inventory acquires only the PM2 process fields required for
// fail-closed correlation. It is package-only until a later handoff slice wires it.
type LinuxPM2Inventory struct {
	Runner    CommandRunner
	Timeout   time.Duration
	MaxOutput int
}

func (i LinuxPM2Inventory) Snapshot(ctx context.Context) ([]PM2Record, error) {
	if i.Runner == nil {
		return nil, errors.New("pm2 inventory unavailable")
	}
	if err := acquisitionContextError(ctx, "pm2 inventory"); err != nil {
		return nil, err
	}
	timeout := i.Timeout
	if timeout <= 0 {
		timeout = defaultPM2AcquisitionTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, _, err := i.Runner.Run(commandCtx, "pm2", "jlist")
	if err != nil {
		if contextErr := acquisitionContextError(commandCtx, "pm2 inventory"); contextErr != nil {
			return nil, wrapObservationUnavailable(contextErr.Error(), err)
		}
		return nil, wrapObservationUnavailable("pm2 inventory command failed", err)
	}
	if contextErr := acquisitionContextError(commandCtx, "pm2 inventory"); contextErr != nil {
		return nil, contextErr
	}
	if len(stdout) > acquisitionOutputLimit(i.MaxOutput) {
		return nil, wrapObservationUnavailable("pm2 inventory output exceeded limit", observationOutputError("pm2-jlist", "sudo -n pm2 jlist", "output-too-large"))
	}
	records, err := ParsePM2Inventory(stdout)
	if err != nil {
		return nil, wrapObservationUnavailable("pm2 inventory output is invalid", observationOutputError("pm2-jlist", "sudo -n pm2 jlist", "output-invalid"))
	}
	return records, nil
}

func acquisitionOutputLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return defaultPM2AcquisitionOutputLimit
}

func acquisitionContextError(ctx context.Context, subject string) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("%s cancelled", subject)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out", subject)
	}
	return nil
}

func ParsePM2Inventory(data []byte) ([]PM2Record, error) {
	var raw []pm2JSONRecord
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return nil, errors.New("pm2 inventory is invalid")
	}
	seenIDs, seenPIDs := map[int64]bool{}, map[int]bool{}
	records := make([]PM2Record, 0, len(raw))
	for _, item := range raw {
		cwd := item.Env.CWD
		if cwd != "" && item.Env.PMCWD != "" && !samePath(cwd, item.Env.PMCWD) {
			return nil, errors.New("pm2 inventory is ambiguous")
		}
		if cwd == "" {
			cwd = item.Env.PMCWD
		}
		pid := item.PID
		if item.PID > 0 && item.Env.PID > 0 && item.PID != item.Env.PID {
			return nil, errors.New("pm2 inventory is ambiguous")
		}
		if pid == 0 {
			pid = item.Env.PID
		}
		execPath := item.ExecPath
		if item.ExecPath != "" && item.Env.Exec != "" && !samePath(item.ExecPath, item.Env.Exec) {
			return nil, errors.New("pm2 inventory is ambiguous")
		}
		if execPath == "" {
			execPath = item.Env.Exec
		}
		if item.ID < 0 || pid <= 0 || item.Name == "" || cwd == "" || execPath == "" || item.Env.Status == "" || seenIDs[item.ID] || seenPIDs[pid] {
			return nil, errors.New("pm2 inventory is ambiguous")
		}
		seenIDs[item.ID], seenPIDs[pid] = true, true
		records = append(records, PM2Record{ID: item.ID, PID: pid, Name: item.Name, CWD: filepath.Clean(cwd), ExecPath: filepath.Clean(execPath), Status: item.Env.Status})
	}
	return records, nil
}
func CorrelatePM2(records []PM2Record, sockets []SocketOwner, proc map[int]ProcIdentity) ([]PM2ProcessIdentity, error) {
	owners := make(map[int]map[uint16]bool, len(sockets))
	ports := make(map[uint16]int, len(sockets))
	for _, socket := range sockets {
		if socket.PID <= 0 || socket.Port == 0 {
			return nil, errors.New("socket ownership is ambiguous")
		}
		if owner, ok := ports[socket.Port]; ok && owner != socket.PID {
			return nil, errors.New("socket ownership is ambiguous")
		}
		ports[socket.Port] = socket.PID
		if owners[socket.PID] == nil {
			owners[socket.PID] = map[uint16]bool{}
		}
		owners[socket.PID][socket.Port] = true
	}
	selected := make([]PM2ProcessIdentity, 0)
	for _, record := range records {
		if record.ID < 0 {
			continue
		}
		var port uint16
		var contract pm2ServiceContract
		matches := 0
		for candidate := range owners[record.PID] {
			if candidateContract, allowed := allowedPM2Contract(record.Name, record.CWD, candidate); allowed {
				port, contract = candidate, candidateContract
				matches++
			}
		}
		if matches == 0 || record.Status != "online" {
			continue
		}
		if matches != 1 {
			return nil, errors.New("socket ownership is ambiguous")
		}
		identity, ok := proc[record.PID]
		if !ok || identity.StartTicks == 0 || record.ExecPath != contract.pm2ExecPath || identity.ExecPath != contract.runtimeExecPath || identity.CWD != contract.cwd {
			return nil, errors.New("process identity is invalid")
		}
		selected = append(selected, PM2ProcessIdentity{PMID: record.ID, Name: record.Name, PID: record.PID, CWD: identity.CWD, ExecPath: record.ExecPath, RuntimeExecPath: identity.ExecPath, Port: port, StartTicks: identity.StartTicks})
	}
	if len(selected) == 0 {
		return nil, errors.New("no qualifying PM2 identity")
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].PMID < selected[j].PMID })
	return selected, nil
}

type pm2ServiceContract struct {
	name, cwd, pm2ExecPath, runtimeExecPath string
	port                                    uint16
}

func allowedPM2Contract(name, cwd string, port uint16) (pm2ServiceContract, bool) {
	contracts := []pm2ServiceContract{
		{"front-guardian", guardianRoot, "/usr/bin/bash", "/usr/local/bin/node", 8080},
		{"ws", backendRoot + "/websocket", "/usr/bin/bash", "/usr/local/bin/node", 4550},
		{"node", backendRoot + "/node", backendRoot + "/node/bin/www", "/usr/local/bin/node", 9090},
	}
	for _, contract := range contracts {
		if name == contract.name && cwd == contract.cwd && port == contract.port {
			return contract, true
		}
	}
	return pm2ServiceContract{}, false
}
func samePath(left, right string) bool { return filepath.Clean(left) == filepath.Clean(right) }

type PM2Controller struct{ Runner CommandRunner }

func (c PM2Controller) Stop(ctx context.Context, identity PM2ProcessIdentity) error {
	if c.Runner == nil || identity.PMID < 0 {
		return errors.New("pm2 stop target is invalid")
	}
	if boundary, ok := c.Runner.(RootPM2Boundary); ok {
		return boundary.mutate(ctx, "stop", identity)
	}
	_, _, err := c.Runner.Run(ctx, "pm2", "stop", strconv.FormatInt(identity.PMID, 10))
	if err != nil {
		return fmt.Errorf("pm2 stop failed")
	}
	return nil
}

func (c PM2Controller) Start(ctx context.Context, identity PM2ProcessIdentity) error {
	if c.Runner == nil || identity.PMID < 0 {
		return errors.New("pm2 recovery target is invalid")
	}
	if boundary, ok := c.Runner.(RootPM2Boundary); ok {
		return boundary.mutate(ctx, "start", identity)
	}
	_, _, err := c.Runner.Run(ctx, "pm2", "start", strconv.FormatInt(identity.PMID, 10))
	if err != nil {
		return errors.New("pm2 recovery command failed")
	}
	return nil
}

type PM2Snapshot struct {
	Records []PM2Record
	Sockets []SocketOwner
	Proc    map[int]ProcIdentity
}
type PM2SnapshotProvider interface {
	Snapshot(context.Context) (PM2Snapshot, error)
}
type PM2Quiescer struct {
	Snapshots         PM2SnapshotProvider
	Controller        PM2Controller
	StopProofTimeout  time.Duration
	StopProofInterval time.Duration
}

func (q PM2Quiescer) Quiesce(ctx context.Context) (PM2Quiescence, error) {
	initial, err := q.snapshot(ctx)
	if err != nil {
		return PM2Quiescence{}, QuiescenceError{Code: "pm2-observation-unavailable", Diagnostic: withObservationStage(err, "initial-snapshot")}
	}
	pending, err := CorrelatePM2(initial.Records, initial.Sockets, initial.Proc)
	if err != nil {
		return PM2Quiescence{}, QuiescenceError{Code: "pm2-correlation-failed"}
	}
	stopped := PM2Quiescence{Processes: append([]PM2ProcessIdentity(nil), pending...)}
	for len(pending) > 0 {
		current, err := q.snapshot(ctx)
		if err != nil {
			return stopped, QuiescenceError{Code: "pm2-observation-unavailable", Diagnostic: withObservationStage(err, "pre-stop-snapshot")}
		}
		active, err := CorrelatePM2(current.Records, current.Sockets, current.Proc)
		if err != nil || !sameIdentities(active, pending) {
			return stopped, QuiescenceError{Code: "pm2-state-changed"}
		}
		target := pending[0]
		if err := q.Controller.Stop(ctx, target); err != nil {
			return stopped, QuiescenceError{Code: "pm2-stop-failed"}
		}
		stopped.Evidence = append(stopped.Evidence, PM2StoppedEvidence{PMID: target.PMID, OriginalPID: target.PID, Port: target.Port, StartTicks: target.StartTicks, StopCommandSucceeded: true})
		if diagnostic := q.waitForStoppedAndReleased(ctx, target); diagnostic != nil {
			return stopped, QuiescenceError{Code: "pm2-stop-unproven", Diagnostic: diagnostic}
		}
		stopped.Evidence[len(stopped.Evidence)-1].StopVerified = true
		pending = pending[1:]
	}
	if final, err := q.snapshot(ctx); err != nil || !stoppedAndReleased(final, stopped.Processes...) {
		return stopped, QuiescenceError{Code: "pm2-final-state-unproven"}
	}
	return stopped, nil
}
func (q PM2Quiescer) Recover(ctx context.Context, stopped PM2Quiescence) (PM2Recovery, error) {
	targets, ok := acknowledgedRecoveryTargets(stopped)
	if !ok || q.Snapshots == nil || q.Controller.Runner == nil {
		return PM2Recovery{Code: "pm2-recovery-invalid"}, errors.New("pm2 recovery is invalid")
	}
	recovery := PM2Recovery{Code: "pm2-recovery-unproven"}
	for index := len(targets) - 1; index >= 0; index-- {
		target := targets[index]
		if before, err := q.snapshot(ctx); err != nil || !recoverySnapshotProves(before, target, false) {
			return recovery, errors.New("pm2 recovery was not proven")
		}
		recovery.Attempted++
		if err := q.Controller.Start(ctx, target); err != nil {
			return recovery, errors.New("pm2 recovery was not proven")
		}
		if after, err := q.snapshot(ctx); err != nil || !recoverySnapshotProves(after, target, true) {
			return recovery, errors.New("pm2 recovery was not proven")
		}
		recovery.Recovered++
	}
	recovery.Verified, recovery.Code = true, "pm2-recovery-verified"
	return recovery, nil
}
func acknowledgedRecoveryTargets(stopped PM2Quiescence) ([]PM2ProcessIdentity, bool) {
	byID := make(map[int64]PM2ProcessIdentity, len(stopped.Processes))
	for _, identity := range stopped.Processes {
		if _, duplicate := byID[identity.PMID]; identity.PMID < 0 || identity.PID <= 0 || identity.Port == 0 || identity.StartTicks == 0 || identity.CWD == "" || identity.ExecPath == "" || identity.RuntimeExecPath == "" || duplicate {
			return nil, false
		}
		byID[identity.PMID] = identity
	}
	targets := make([]PM2ProcessIdentity, 0, len(stopped.Evidence))
	seen := make(map[int64]bool, len(stopped.Evidence))
	for _, evidence := range stopped.Evidence {
		identity, ok := byID[evidence.PMID]
		if !ok || seen[evidence.PMID] || (!evidence.StopCommandSucceeded && !evidence.StopVerified) || identity.PID != evidence.OriginalPID || identity.Port != evidence.Port || identity.StartTicks != evidence.StartTicks {
			return nil, false
		}
		seen[evidence.PMID] = true
		targets = append(targets, identity)
	}
	return targets, len(targets) > 0
}
func recoverySnapshotProves(snapshot PM2Snapshot, target PM2ProcessIdentity, started bool) bool {
	record, ok := exactRecoveryRecord(snapshot.Records, target)
	if !ok {
		return false
	}
	owners := 0
	for _, socket := range snapshot.Sockets {
		if socket.Port == target.Port {
			owners++
			if socket.PID != record.PID {
				return false
			}
		}
	}
	if !started {
		return record.Status == "stopped" && owners == 0
	}
	identity, ok := snapshot.Proc[record.PID]
	return record.Status == "online" && record.PID > 0 && record.PID != target.PID && owners == 1 && ok && identity.StartTicks != 0 && identity.StartTicks != target.StartTicks && identity.CWD == target.CWD && identity.ExecPath == target.RuntimeExecPath
}
func exactRecoveryRecord(records []PM2Record, target PM2ProcessIdentity) (PM2Record, bool) {
	var match PM2Record
	found := false
	for _, record := range records {
		if record.ID != target.PMID {
			continue
		}
		if found || record.Name != target.Name || !samePath(record.CWD, target.CWD) || !samePath(record.ExecPath, target.ExecPath) {
			return PM2Record{}, false
		}
		match = record
		found = true
	}
	return match, found
}
func (q PM2Quiescer) snapshot(ctx context.Context) (PM2Snapshot, error) {
	if q.Snapshots == nil || q.Controller.Runner == nil {
		return PM2Snapshot{}, errors.New("pm2 quiescer is unavailable")
	}
	return q.Snapshots.Snapshot(ctx)
}
func (q PM2Quiescer) waitForStoppedAndReleased(ctx context.Context, target PM2ProcessIdentity) *PM2ObservationDiagnostic {
	timeout := q.StopProofTimeout
	if timeout <= 0 {
		timeout = defaultPM2StopProofTimeout
	}
	interval := q.StopProofInterval
	if interval <= 0 {
		interval = defaultPM2StopProofInterval
	}
	proofCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var observationFailure *PM2ObservationDiagnostic
	for {
		after, err := q.snapshot(proofCtx)
		if err == nil && proofCtx.Err() == nil && stoppedAndReleased(after, target) {
			return nil
		}
		if err != nil && observationFailure == nil {
			observationFailure = withObservationStage(err, "stop-proof-snapshot")
		}
		timer := time.NewTimer(interval)
		select {
		case <-proofCtx.Done():
			timer.Stop()
			if observationFailure != nil {
				return observationFailure
			}
			return &PM2ObservationDiagnostic{
				StopProofTimedOut:  errors.Is(proofCtx.Err(), context.DeadlineExceeded),
				StopProofCancelled: !errors.Is(proofCtx.Err(), context.DeadlineExceeded),
				PMID:               target.PMID,
				Port:               target.Port,
			}
		case <-timer.C:
		}
	}
}
func sameIdentities(current, expected []PM2ProcessIdentity) bool {
	if len(current) != len(expected) {
		return false
	}
	for i := range current {
		if current[i] != expected[i] {
			return false
		}
	}
	return true
}
func stoppedAndReleased(snapshot PM2Snapshot, targets ...PM2ProcessIdentity) bool {
	for _, target := range targets {
		found := false
		for _, record := range snapshot.Records {
			if record.ID == target.PMID {
				if found || record.Status != "stopped" || !samePath(record.CWD, target.CWD) || !samePath(record.ExecPath, target.ExecPath) {
					return false
				}
				found = true
			}
		}
		if !found {
			return false
		}
		for _, socket := range snapshot.Sockets {
			if socket.Port == target.Port {
				return false
			}
		}
	}
	return true
}
