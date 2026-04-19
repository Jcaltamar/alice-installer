package compose_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/compose"
	"github.com/jcaltamar/alice-installer/internal/platform"
)

// TestCLICompose_Pull_InvokesDockerComposeNotDocker is a regression test for the
// bug where baseArgs omitted the "compose" subcommand and the installer shelled
// out to `docker -f ...` which docker rejects with "unknown shorthand flag: 'f'".
// The invocation MUST start with `docker compose`.
func TestCLICompose_Pull_InvokesDockerComposeNotDocker(t *testing.T) {
	streamer := &platform.FakeStreamingCommandRunner{}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.PullProgressMsg, 1)
	_ = c.Pull(context.Background(), []string{"docker-compose.yml"}, ".env", ch)

	if streamer.LastName != "docker" {
		t.Fatalf("LastName = %q, want %q", streamer.LastName, "docker")
	}
	if len(streamer.LastArgs) == 0 || streamer.LastArgs[0] != "compose" {
		t.Fatalf("first arg = %q, want %q. Full args: %v",
			firstOr(streamer.LastArgs, "<empty>"), "compose", streamer.LastArgs)
	}
	joined := strings.Join(append([]string{streamer.LastName}, streamer.LastArgs...), " ")
	if !strings.Contains(joined, "docker compose -f docker-compose.yml") {
		t.Errorf("invocation = %q, want prefix 'docker compose -f docker-compose.yml'", joined)
	}
	if !strings.Contains(joined, "pull") {
		t.Errorf("invocation = %q, missing 'pull' subcommand", joined)
	}
}

// TestCLICompose_Up_InvokesDockerCompose — same regression guard for Up.
func TestCLICompose_Up_InvokesDockerCompose(t *testing.T) {
	streamer := &platform.FakeStreamingCommandRunner{}
	c := compose.NewCLICompose(nil, streamer)

	ch := make(chan compose.UpProgressMsg, 1)
	_ = c.Up(context.Background(), []string{"docker-compose.yml"}, ".env", ch)

	if len(streamer.LastArgs) == 0 || streamer.LastArgs[0] != "compose" {
		t.Fatalf("Up first arg = %q, want %q. Full args: %v",
			firstOr(streamer.LastArgs, "<empty>"), "compose", streamer.LastArgs)
	}
}

func firstOr(ss []string, fallback string) string {
	if len(ss) == 0 {
		return fallback
	}
	return ss[0]
}
