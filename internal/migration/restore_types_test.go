package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRestoreSafetyContractsAreSecretFree(t *testing.T) {
	result := RestoreResult{Outcome: RestorePartialCutover, FailedStage: StageRestoreValidation, Mutated: true, Rollback: RollbackFailed, LegacyBackup: BackupEvidence{Validated: true}, Code: "restore-validation"}
	if RestoreSucceeded != 0 || StagePlatformGate != 0 || RollbackNotRequired != 0 {
		t.Fatal("zero values must be stable")
	}
	data, _ := json.Marshal(result)
	for _, rendered := range []string{fmt.Sprint(result), fmt.Sprintf("%+v", result), string(data)} {
		if strings.Contains(rendered, "slice-1-secret") {
			t.Fatalf("secret leaked: %s", rendered)
		}
	}
}
func TestWaitersRecordDurationAndCancellation(t *testing.T) {
	fake := &FakeWaiter{}
	if err := fake.Wait(context.Background(), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(fake.Durations) != 1 || fake.Durations[0] != 60*time.Second {
		t.Fatalf("durations = %v", fake.Durations)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (RealWaiter{}).Wait(ctx, time.Hour); err == nil {
		t.Fatal("cancelled waiter succeeded")
	}
}
