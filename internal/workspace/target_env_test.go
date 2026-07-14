package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const targetSecret = "slice-1-secret"

func TestTargetEnvReaderAcceptsOnlyGeneratedDatabaseFields(t *testing.T) {
	path := writeTargetEnv(t, "# generated\nPOSTGRES_HOST=127.0.0.1\nPOSTGRES_PORT=5432\nPOSTGRES_USER=alice\nPOSTGRES_PASSWORD="+targetSecret+"\nPOSTGRES_DATABASE=alice\nUNRELATED=value\n")
	got, err := (TargetEnvFileReader{}).ReadTargetDatabase(context.Background(), path)
	if err != nil || got.Host != "127.0.0.1" || got.Port != 5432 || got.User != "alice" || got.Database != "alice" {
		t.Fatalf("config=%+v err=%v", got, err)
	}
	data, _ := json.Marshal(got)
	if strings.Contains(got.String()+string(data), targetSecret) {
		t.Fatal("password leaked")
	}
}
func TestTargetEnvReaderRejectsUnsafeInputWithoutLeakingSecrets(t *testing.T) {
	valid := "POSTGRES_HOST=127.0.0.1\nPOSTGRES_PORT=5432\nPOSTGRES_USER=alice\nPOSTGRES_PASSWORD=" + targetSecret + "\nPOSTGRES_DATABASE=alice\n"
	cases := []struct{ name, content, code string }{
		{"missing", strings.Replace(valid, "POSTGRES_PASSWORD="+targetSecret+"\n", "", 1), "target-env-missing-key"}, {"empty", strings.Replace(valid, "POSTGRES_USER=alice", "POSTGRES_USER=", 1), "target-env-empty-key"}, {"duplicate", valid + "POSTGRES_HOST=127.0.0.1\n", "target-env-duplicate-key"}, {"syntax", "export POSTGRES_HOST=127.0.0.1\n" + valid, "target-env-malformed"}, {"quoted", strings.Replace(valid, "POSTGRES_USER=alice", "POSTGRES_USER=\"alice\"", 1), "target-env-malformed"}, {"host", strings.Replace(valid, "127.0.0.1", "localhost", 1), "target-env-invalid-host"}, {"port", strings.Replace(valid, "5432", "05432", 1), "target-env-invalid-port"}, {"user", strings.Replace(valid, "alice", "bad-name", 1), "target-env-invalid-user"}, {"database", strings.Replace(valid, "POSTGRES_DATABASE=alice", "POSTGRES_DATABASE=postgres", 1), "target-env-invalid-database"}, {"escape", strings.Replace(valid, "POSTGRES_USER=alice", "POSTGRES_USER=alice\\x", 1), "target-env-malformed"}, {"substitution", strings.Replace(valid, "POSTGRES_USER=alice", "POSTGRES_USER=$USER", 1), "target-env-malformed"}, {"inline comment", strings.Replace(valid, "POSTGRES_USER=alice", "POSTGRES_USER=alice # comment", 1), "target-env-malformed"}, {"multiline", valid + "broken-line\n", "target-env-malformed"}, {"nul", valid + "\x00", "target-env-malformed"}, {"cr", valid + "\r", "target-env-malformed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (TargetEnvFileReader{}).ReadTargetDatabase(context.Background(), writeTargetEnv(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.code) || strings.Contains(err.Error(), targetSecret) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestTargetEnvReaderRejectsSpecialFilesAndBounds(t *testing.T) {
	path := writeTargetEnv(t, strings.Repeat("x", targetEnvMaxBytes+1))
	if _, err := (TargetEnvFileReader{}).ReadTargetDatabase(context.Background(), path); err == nil || !strings.Contains(err.Error(), "target-env-too-large") {
		t.Fatalf("err=%v", err)
	}
	link := filepath.Join(t.TempDir(), ".env")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (TargetEnvFileReader{}).ReadTargetDatabase(context.Background(), link); err == nil || !strings.Contains(err.Error(), "target-env-unsafe-file") {
		t.Fatalf("err=%v", err)
	}
}
func writeTargetEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
