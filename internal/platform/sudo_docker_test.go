package platform

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSudoDockerCommandRunner(t *testing.T) {
	base := &FakeCommandRunner{}
	runner := SudoDockerCommandRunner{Runner: base}

	if _, _, err := runner.Run(context.Background(), "docker", "compose", "pull"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if base.LastName != "sudo" || !reflect.DeepEqual(base.LastArgs, []string{"-n", "docker", "compose", "pull"}) {
		t.Fatalf("invocation = %q %v", base.LastName, base.LastArgs)
	}
	if _, _, err := runner.Run(context.Background(), "sh", "-c", "docker ps"); !errors.Is(err, ErrNonDockerCommand) {
		t.Fatalf("non-Docker error = %v, want %v", err, ErrNonDockerCommand)
	}
}

func TestSudoDockerStreamingCommandRunner(t *testing.T) {
	base := &FakeStreamingCommandRunner{Lines: []string{"pulling"}}
	runner := SudoDockerStreamingCommandRunner{Runner: base}
	var lines []string
	if err := runner.Stream(context.Background(), func(line string) { lines = append(lines, line) }, func(string) {}, "docker", "compose", "up", "--detach"); err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if base.LastName != "sudo" || !reflect.DeepEqual(base.LastArgs, []string{"-n", "docker", "compose", "up", "--detach"}) {
		t.Fatalf("invocation = %q %v", base.LastName, base.LastArgs)
	}
	if !reflect.DeepEqual(lines, []string{"pulling"}) {
		t.Fatalf("lines = %v", lines)
	}
	if err := runner.Stream(context.Background(), func(string) {}, func(string) {}, "bash"); !errors.Is(err, ErrNonDockerCommand) {
		t.Fatalf("non-Docker error = %v, want %v", err, ErrNonDockerCommand)
	}
}
