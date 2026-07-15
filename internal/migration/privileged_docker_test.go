package migration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSudoDockerExecutorUsesFixedArgvAndRejectsOtherExecutables(t *testing.T) {
	fake := &recordingExecutor{results: []ProcessResult{{Outcome: ProcessSucceeded}}}
	executor := SudoDockerExecutor{Executor: fake}
	result := executor.Run(context.Background(), ProcessSpec{Name: "docker", Args: []string{"ps", "--all"}}, io.Discard)
	if result.Outcome != ProcessSucceeded || len(fake.specs) != 1 {
		t.Fatalf("result/specs = %#v/%#v", result, fake.specs)
	}
	want := ProcessSpec{Name: "sudo", Args: []string{"-n", "docker", "ps", "--all"}}
	if fake.specs[0].Name != want.Name || !equalStrings(fake.specs[0].Args, want.Args) {
		t.Fatalf("argv = %#v, want %#v", fake.specs[0], want)
	}
	for _, name := range []string{"sh", "bash", "sudo", "docker compose"} {
		if got := executor.Run(context.Background(), ProcessSpec{Name: name, Args: []string{"anything"}}, io.Discard); got.StderrCode != "process-precondition" || len(fake.specs) != 1 {
			t.Fatalf("%q was not rejected: %#v", name, got)
		}
	}
}

func TestSudoDockerExecutorReturnsStablePermissionCode(t *testing.T) {
	fake := &recordingExecutor{results: []ProcessResult{{Outcome: ProcessFailed, StderrCode: SudoDockerPermissionCode}, {Outcome: ProcessFailed, StderrCode: SudoDockerPermissionCode}}}
	executor := SudoDockerExecutor{Executor: fake}
	if got := executor.Run(context.Background(), ProcessSpec{Name: "docker", Args: []string{"inspect", strings.Repeat("a", 64)}}, io.Discard); got.StderrCode != SudoDockerPermissionCode {
		t.Fatalf("result = %#v", got)
	}
	if _, _, err := executor.RunDocker(context.Background(), "docker", "ps"); !errors.Is(err, ErrSudoDockerPermission) {
		t.Fatalf("RunDocker() error = %v", err)
	}
}

func TestDispositionCommandsTargetFullIDAndNeverDeleteVolumes(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, disposition := range []ContainerDisposition{DispositionStop, DispositionRemove} {
		t.Run(map[ContainerDisposition]string{DispositionStop: "stop", DispositionRemove: "remove"}[disposition], func(t *testing.T) {
			fake := &recordingExecutor{}
			controller := DockerLegacyContainerController{Executor: fake}
			if _, err := controller.Apply(context.Background(), id, disposition); err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if disposition == DispositionRemove {
				wantCalls = 2
			}
			if len(fake.specs) != wantCalls {
				t.Fatalf("calls = %#v", fake.specs)
			}
			assertDispositionArgvSafe(t, id, fake.specs)
		})
	}
}

func TestRemoveFailureReportsStoppedContainerForRecovery(t *testing.T) {
	id := strings.Repeat("d", 64)
	fake := &recordingExecutor{results: []ProcessResult{{Outcome: ProcessSucceeded}, {Outcome: ProcessFailed}}}
	result, err := (DockerLegacyContainerController{Executor: fake}).Apply(context.Background(), id, DispositionRemove)
	if err == nil || result.Code != DispositionStoppedCode || !result.Verified {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	assertDispositionArgvSafe(t, id, fake.specs)
}

func TestStopDispositionRecoveryStartsAndVerifiesSameContainer(t *testing.T) {
	id := strings.Repeat("b", 64)
	fake := &recordingExecutor{stdout: []byte("true\n")}
	result, err := (DockerLegacyContainerController{Executor: fake}).Recover(context.Background(), id, DispositionStop)
	if err != nil || !result.Verified || result.Code != DispositionRestartedCode {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if len(fake.specs) != 2 || !equalStrings(fake.specs[0].Args, []string{"start", id}) || !equalStrings(fake.specs[1].Args, []string{"inspect", "--format", "{{.State.Running}}", id}) {
		t.Fatalf("recovery argv = %#v", fake.specs)
	}
	assertDispositionArgvSafe(t, id, fake.specs)
}

func TestRemoveDispositionRecoveryIsManualAndRunsNoCommand(t *testing.T) {
	fake := &recordingExecutor{}
	result, err := (DockerLegacyContainerController{Executor: fake}).Recover(context.Background(), strings.Repeat("c", 64), DispositionRemove)
	if err != nil || result.Code != DispositionManualRecoveryCode || len(fake.specs) != 0 {
		t.Fatalf("result/error/specs = %#v/%v/%#v", result, err, fake.specs)
	}
}

func assertDispositionArgvSafe(t *testing.T, id string, specs []ProcessSpec) {
	t.Helper()
	for _, spec := range specs {
		if spec.Name != "docker" || spec.Args[len(spec.Args)-1] != id {
			t.Fatalf("unsafe target: %#v", spec)
		}
		joined := strings.Join(spec.Args, " ")
		for _, forbidden := range []string{"-v", "--volumes", "volume rm", "prune", "down -v"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("forbidden token %q in %#v", forbidden, spec)
			}
		}
	}
}

type recordingExecutor struct {
	specs   []ProcessSpec
	results []ProcessResult
	stdout  []byte
}

func (f *recordingExecutor) Run(_ context.Context, spec ProcessSpec, stdout io.Writer) ProcessResult {
	f.specs = append(f.specs, spec)
	if len(f.stdout) > 0 {
		_, _ = bytes.NewReader(f.stdout).WriteTo(stdout)
	}
	if len(f.results) == 0 {
		return ProcessResult{Outcome: ProcessSucceeded}
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result
}
