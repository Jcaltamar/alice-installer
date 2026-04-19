# Archived Changes

Changes whose implementation is already merged into `main`. The folders
preserve the original SDD artifacts (proposal, design, specs, tasks) for
historical reference.

| Change | Shipped in | Key commits |
|--------|-----------|-------------|
| `installer-tui` | v0.1.0 | Base installer TUI — preflight, workspace input, port scan, env-write, pull, deploy, verify |
| `installer-bootstrap` | v0.1.0 | `3d524cb` — auto-elevate `/opt/alice-*` dir creation via sudo |
| `installer-docker-bootstrap` | v0.1.0 | `4e63f33` — Docker install + `systemctl enable --now docker` + `usermod -aG docker` actions |

Later work (v0.2.x–v0.3.0) — WorkspaceDir split, `--unattended` headless
mode, E2E harness, golden snapshots, queue service removal, compose
subcommand fix, pull/deploy drain loop fix — was done directly in git
without a tracked SDD change. The commit history is the source of truth
for that work; see `git log --oneline v0.1.0..v0.3.0`.
