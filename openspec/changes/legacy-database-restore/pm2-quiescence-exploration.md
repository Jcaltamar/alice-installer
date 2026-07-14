# Exploration Delta: Selective PM2 Legacy Quiescence

## Decision summary

Add a distinct, fail-closed PM2 quiescence capability to the existing interactive Linux migration restore boundary. It must run **after the legacy backup has been revalidated** and **before the existing new installation starts**. It must not be folded into the existing detection probe: detection is observational and currently returns only presence/evidence, while quiescence mutates process state and requires identity, socket, rollback, and race handling.

Only PM2 records satisfying both exact directory relationship and listening TCP port qualify:

| Legacy root / cwd relationship | Required TCP port |
| --- | ---: |
| `/opt/alice-guardian` | `8080` |
| `/opt/backend_alice_guardian` | `9090` or `4550` |

No process qualifies from a name, script basename, executable path, or directory alone. Never use `pm2 stop all`; never stop unrelated PM2 processes. Ambiguous, incomplete, stale, contradictory, or racing evidence fails closed.

## Existing abstractions and exact seams

- Existing PM2 observation: `internal/installation/pm2_probe.go`, tests in `internal/installation/pm2_probe_test.go`.
  - `PM2Probe` invokes direct argv `pm2 jlist` through `installation.CommandRunner`.
  - Current `pm2Record` contains `name`, `pm_exec_path`, `pm2_env.cwd`, and `pm2_env.pm_exec_path`, but no PID or port evidence.
  - `LegacyPolicy.matches` is name/script/path-oriented and is unsuitable for this delta because the new rule requires a conjunction of canonical cwd relationship and socket ownership.
  - Do not mutate `PM2Probe`/`LegacyPolicy` semantics used by contextual detection. A new package-level capability should reuse only the direct command-runner pattern and Linux platform gate.
- Existing migration coordinator and seams: `internal/migration/restore.go`, `restore_types.go`, `restore_adapters.go`, `restore_composition.go`, with tests beside each. `LegacyRestoreAction` is reached by the interactive TUI after deploy; this delta changes the pre-install handoff, not the database restore sequence itself.
- Existing Compose direct-argv/service boundary: `internal/compose/runner.go`, `fake.go`, tests. It is unrelated to PM2 and must not be used for PM2 stop/start.
- Existing route wiring: `cmd/installer/main.go`; interactive-only construction is the safe production composition seam. `internal/tui/model.go` and `internal/tui/migration_restore.go` own the pending migration handoff/state. `internal/headless` must remain unchanged and must not receive this capability.
- Existing platform gate: `internal/platform`/`installation.Platform`; support remains Linux `amd64` and `arm64` only.

The existing OpenSpec restore artifacts already use direct-argv executors, typed redacted evidence, explicit rollback, and strict TDD. Extend those patterns rather than adding shell commands or raw error output.

## Proposed capability contract

Names are implementation targets, not an instruction to force one file layout:

```go
type LegacyPM2Quiescer interface {
    Quiesce(ctx context.Context, request PM2QuiesceRequest) (PM2Quiescence, error)
    Recover(ctx context.Context, stopped PM2Quiescence) (PM2Recovery, error)
}

type PM2QuiesceRequest struct {
    GOOS, GOARCH string
    // authoritative host/workspace context only; no caller-supplied shell text
}

type PM2ProcessIdentity struct {
    PMID       int64
    Name       string // bounded, redacted-safe identity label if needed
    PID        int
    CWD        string
    ExecPath   string
    Port       uint16
    StartTicks uint64 // preferred Linux process incarnation proof
}

type PM2Quiescence struct {
    OperationID string
    Processes  []PM2ProcessIdentity // exact stopped set, deterministic order
    Evidence   []PM2StoppedEvidence // no secrets/raw output
}
```

The returned stopped set must be immutable to callers (copy on construction). Preserve PM2 id, PID, canonical cwd, executable path, matched port, and process-incarnation evidence. Names are not sufficient identity because PM2 names can be reused.

## Safe correlation method: PM2 records, PIDs, cwd/exec, and sockets

Use a single bounded acquisition snapshot immediately before stopping:

1. Invoke `pm2 jlist` directly. Extend the private JSON shape only for fields needed to establish identity: PM2 id (`pm_id`), process PID (`pid` or `pm2_env.pid` depending on PM2 output), `pm2_env.cwd`, `pm_exec_path`/`pm2_env.pm_exec_path`, and PM2 status. Reject missing, duplicate, invalid, non-running, or contradictory identity fields; do not accept a partial valid subset.
2. Acquire Linux socket ownership using a dedicated direct-argv adapter, preferably `ss -H -ltnp` (or a fixed reviewed equivalent) and a strict parser. Do not use `lsof` unless the binary/format contract is explicitly pinned and tested. Parse only listening TCP rows, local port, and owning PID(s). Reject malformed rows, duplicate ownership, inaccessible process metadata, and a port with ambiguous owners.
3. Correlate on **PID plus process incarnation**, not name. For each PM2 record, require:
   - PM2 status is running;
   - PID is positive and unique in the PM2 snapshot;
   - `/proc/<pid>/cwd` resolves to the exact configured root or a descendant using `filepath.Rel` (root itself allowed; prefix collisions such as `/opt/alice-guardian-old` rejected);
   - `/proc/<pid>/exe` or the PM2 exec path is captured as evidence and, when available, agrees with the PM2 record; disagreement fails closed;
   - the socket snapshot shows the same PID listening on the required TCP port; a PM2 PID listening on a wrong port does not match;
   - read `/proc/<pid>/stat` start time (`starttime`) before and immediately before stop, or equivalent stable Linux incarnation proof. PID reuse or changed start time invalidates the snapshot.
4. Apply exact rules: `/opt/alice-guardian`→8080; `/opt/backend_alice_guardian`→9090/4550. A process matching either directory but no required port is not selected. A required port owned by multiple candidate PIDs, or a candidate PID owning multiple contradictory records, is ambiguity and aborts the whole operation.
5. Revalidate the complete selected set and socket/cwd/incarnation evidence immediately before each stop action. If any selected process disappears, changes cwd/exe/status/port/start ticks, or a new competing owner appears, stop nothing further and enter recovery for only processes already stopped.

Do not infer socket ownership from `pm2 jlist` environment variables. Do not correlate by PM2 name, script basename, port alone, or directory alone.

## Stop/start command boundary

Use a dedicated `PM2Controller` backed by `installation.CommandRunner` or the repository's equivalent direct `BinaryExecutor`; keep argv as separate tokens:

- Stop exactly one selected process at a time: `pm2 stop <pm_id>` (or a fixed `pm2 stop <validated-pm-id>` builder). Never construct a shell string and never pass a list or `all`.
- After each successful stop, re-query PM2 and verify that the exact PM2 id/PID is stopped and that no unrelated selected/non-selected process was affected. A command success without matching postcondition is failure/ambiguity.
- Recovery starts exactly the identities recorded as successfully stopped, one at a time: `pm2 start <pm_id>`; never `pm2 resurrect`, `pm2 restart all`, `pm2 start all`, or name-only restart. If PM2 id reuse is detected, do not start it.
- Verify recovery with a fresh PM2 record, same process incarnation/identity where PM2 preserves it, same cwd/exec relationship, and the expected listening port. If the original process was replaced, the port is owned by another PID, or evidence is ambiguous, do not start anything else and report bounded recovery failure.

PM2 ids, PIDs, ports, roots, and start ticks may be persisted as bounded evidence; no command stderr, environment dump, password, or arbitrary command output may be persisted.

## Coordinator placement and semantics

The quiescence boundary belongs in the interactive migration handoff, after validated legacy backup revalidation and before `Compose.Up`/the existing new installation begins. The required sequence is:

`platform/request gate → legacy backup validation → PM2 snapshot/correlation → selective stop + postconditions → existing installation unchanged`.

On any pre-install quiescence failure, abort before installation side effects. If installation/restore fails before successful final cutover, invoke `Recover` for exactly the successfully stopped set, then verify recovery. On successful migration, do not recover; leave the exact legacy set stopped. A failure after quiescence but before final cutover must not accidentally restart unrelated PM2 processes.

Integrate with the existing restore result/state model by adding a stage/code for PM2 quiescence and bounded evidence, or by carrying a private pre-install outcome into the existing migration terminal result. Do not overload database `Mutated`: PM2 mutation is a separate state. The existing database rollback remains responsible for target data; PM2 recovery is a separate compensating action and must be attempted before reporting a pre-cutover failure.

A cancellation or timeout follows the same rule: if any PM2 process was stopped, recover only that exact set under a bounded recovery context. If recovery cannot be proven, fail closed with the PM2 set retained in evidence and do not continue installation.

## Race/TOCTOU controls

- Snapshot and stop are inherently racy; no observation can authorize an unbounded later command. Re-read PM2, sockets, cwd/exe, and start ticks at the stop boundary.
- Stop one process per validated identity, revalidate the remaining set after every stop, and abort on drift.
- Record successful stop acknowledgements only after post-stop identity verification. A failed/ambiguous stop is not added to the recovery set unless the postcondition proves it stopped.
- Before proceeding to installation, perform one final complete check that every intended legacy identity is stopped and no unrelated target was stopped. If a required process respawns under a new PID, fail closed; do not chase the replacement automatically.
- Recovery must verify PM2 id/PID/start ticks before each start to prevent PM2 id or PID reuse. Recovery is best effort but bounded, non-retrying, and never broad.
- Linux-only `/proc` and `ss` adapters must reject permission errors and missing records rather than treating them as absence. Use bounded output sizes, fixed timeouts, and direct argv.

## Test seams and required tests

Likely new files: `internal/installation/pm2_quiescence.go` and `pm2_quiescence_test.go`, or `internal/migration/pm2_quiescence.go` if the coordinator owns the capability. Prefer a small `internal/installation` acquisition package plus migration orchestration tests so detection remains unchanged. Add focused fake seams for:

- PM2 command runner recording exact argv and returning controlled JSON;
- Linux socket snapshot provider;
- `/proc` cwd, exe, and start-ticks provider;
- clock/operation-ID provider if evidence needs deterministic identity;
- PM2 controller and migration coordinator recording calls.

Required strict-TDD cases:

- Positive exact matches for all three rules; descendant cwd accepted, root-prefix collision rejected.
- Directory-only, port-only, wrong-port, wrong-cwd, wrong-exe, wrong-status, unrelated PM2, and unrelated listener cases select nothing or fail closed as appropriate.
- Missing/duplicate/invalid PM2 id/PID/start ticks; malformed/mixed PM2 JSON; malformed or ambiguous `ss` output; permission/timeout/cancellation failures.
- PID reuse, changed cwd/exe, changed port owner, respawn between snapshot and stop, process disappearance, and drift after one of several stops.
- Exact argv proves `pm2 stop <id>`/`pm2 start <id>` only; tests reject `all`, broad restart/resurrect, shell execution, interpolation, and unrelated service control.
- Partial stop set recovery starts only acknowledged stopped identities; recovery failure/ambiguity leaves remaining processes untouched and is redacted.
- Installation is not started unless quiescence fully succeeds; pre-cutover failure/cancellation recovers and verifies; successful final cutover leaves legacy processes stopped; restore failure invokes only recorded PM2 recovery.
- Existing detection tests remain unchanged and ordinary Install/Update/Restart/dry-run/unattended/headless routes receive no quiescer.

## Linux/platform notes

The feature is Linux amd64/arm64 only, matching existing PM2 probing and restore support. Windows and other platforms must return unsupported before PM2 or socket commands. `/proc` paths and `ss` output are host-Linux implementation details; do not add a Windows fallback or use broad process APIs.

## Review/workload forecast

A focused quiescence slice is approximately **280–360 authored changed lines** including production adapters, coordinator seam, and strict-TDD tests; keep it below the 400-line review budget. If the existing restore handoff requires more than roughly 100 lines of orchestration, split into:

1. PM2 snapshot/correlation/controller and tests (about 180–240 lines).
2. Migration boundary, compensating recovery, route regressions, and tests (about 100–140 lines).

Do not modify the already-completed database replacement/process boundary or documentation in the first slice. This is a behavior/state/process integration change, so the dominant review lens is reliability; resilience is also materially relevant if the slices are reviewed separately under the repository's bounded-review policy.

## Open questions / implementation hazards

- Confirm the exact PM2 `jlist` PID field and whether PM2 `stop <pm_id>` preserves enough metadata for safe `start <pm_id>` recovery on the supported PM2 version. If not, require a fixed ecosystem/script identity contract and fail closed rather than starting by name.
- Confirm `ss -H -ltnp` output availability/permissions in the target installer environment. No fallback should be added without an equally strict parser and ownership proof.
- Confirm the precise point represented by “before new installation starts” in the current interactive flow: the quiescer must precede the first installation side effect, not merely precede database restore after `Compose.Up`.
