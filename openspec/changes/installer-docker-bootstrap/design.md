# Design: installer-docker-bootstrap

## BootstrapEnv

New file: `internal/tui/bootstrap_env.go`

```go
type BootstrapEnv struct {
    UserName            string // from CurrentUserFn(); fallback $USER; error path on both empty
    DockerBinaryPresent bool   // LookPathFn("docker") == nil
    UserInDockerGroup   bool   // docker group exists AND user's GIDs include it
    SystemdPresent      bool   // LookPathFn("systemctl") == nil AND StatFn("/run/systemd/system") == nil
}
```

### Test seams (injectable function fields)

```go
type envDetector struct {
    LookPathFn    func(string) (string, error)
    StatFn        func(string) (os.FileInfo, error)
    CurrentUserFn func() (*user.User, error)
    LookupGroupFn func(string) (*user.Group, error)
}
```

`DetectEnv()` uses a production `envDetector` with real stdlib functions. Tests instantiate a custom `envDetector` directly and call its `detect()` method.

### UserInDockerGroup logic

1. `LookupGroupFn("docker")` → get group GID (string).
2. `CurrentUserFn()` → get `*user.User`.
3. `u.GroupIds()` → []string of GIDs.
4. If docker group GID is in that slice → true.
5. Any error → false (fail-safe: do not claim membership on error).

### SystemdPresent logic

Both conditions must hold:
- `LookPathFn("systemctl")` returns no error
- `StatFn("/run/systemd/system")` returns no error

### UserName resolution

1. `CurrentUserFn()` → `u.Username` if non-empty.
2. Fallback: `os.Getenv("USER")`.
3. If still empty: use literal `"$USER"` (same as existing code in `buildDirAction`).

---

## Updated ClassifyBlockers Signature

**BREAKING** — adds `env BootstrapEnv` as second parameter, removes `mediaDir, configDir string`.

```go
func ClassifyBlockers(report preflight.Report, env BootstrapEnv, mediaDir, configDir string) (fixable []Action, nonFixable []preflight.CheckResult)
```

Wait — per implementation decision: keeping `mediaDir, configDir` as well (they are still needed for dir actions). Final signature:

```go
func ClassifyBlockers(report preflight.Report, env BootstrapEnv, mediaDir, configDir string) (fixable []Action, nonFixable []preflight.CheckResult)
```

All existing callers: `model.go` (passes `m.deps.Env, m.deps.MediaDir, m.deps.ConfigDir`) and all test files.

---

## Action Priority Table

| Priority | Action ID            | Trigger condition                                                   |
|----------|----------------------|---------------------------------------------------------------------|
| 1        | `docker_install`     | CheckDockerDaemon=FAIL + DockerBinaryPresent=false                  |
| 2        | `media_writable`     | CheckMediaWritable=FAIL                                             |
| 2        | `config_writable`    | CheckConfigWritable=FAIL                                            |
| 3        | `systemd_start_docker` | CheckDockerDaemon=FAIL + Binary=true + InGroup=true + Systemd=true |
| 4        | `docker_group_add`   | CheckDockerDaemon=FAIL + Binary=true + InGroup=false               |

Dir actions retain their natural order (media before config) from the report iteration order. Docker install is always prepended, group-add is always appended.

Implementation: build 4 buckets, concatenate: `[install] + [dirs] + [systemctl] + [usermod]`.

---

## Action Struct: PostActionBanner

Add to `messages.go` Action struct:

```go
type Action struct {
    ID               string
    Description      string
    Command          string
    Args             []string
    PostActionBanner string // optional; non-empty → show interstitial after bootstrap completes
}
```

Zero value = no banner (backward compatible; existing tests unaffected).

---

## BootstrapModel: Banner Screen

Add field `showingBanner bool` to `BootstrapModel`. After the last action succeeds:

1. Collect all `PostActionBanner` values from `m.actions` where the corresponding result was `Err=nil`.
2. If any non-empty banners → set `m.showingBanner = true`, store banners in `m.banners []string`.
3. In `Update`: when `m.showingBanner` and `tea.KeyEnter` → emit `BootstrapCompleteMsg`.
4. In `View`: when `m.showingBanner` → render banners in `theme.Warning`, show "Press Enter to continue".

---

## Dependencies Struct: Env field

```go
type Dependencies struct {
    // ... existing fields ...
    Env BootstrapEnv // populated via tui.DetectEnv() in newDependencies
}
```

`model.go` ClassifyBlockers call becomes:
```go
fixable, nonFixable := ClassifyBlockers(msg.Report, m.deps.Env, m.deps.MediaDir, m.deps.ConfigDir)
```

---

## Routing: model.go

No state machine changes needed. The `Env` field is populated at startup; `ClassifyBlockers` receives it. The banner screen is internal to `BootstrapModel` — the root model sees only `BootstrapCompleteMsg` (same as before).
