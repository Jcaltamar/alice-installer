package asterisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileResourceStoreCreatesSecureBackendVisibleBundle(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "asterisk")
	contract := AMIContract{Enabled: true, Host: "127.0.0.1", Port: 5038, Username: "guardian", Password: "secret", ConfigDir: root}
	store := FileResourceStore{}

	if err := store.CreateBundle(root, contract); err != nil {
		t.Fatalf("CreateBundle() returned error: %v", err)
	}

	dirs := []string{"templates", "sounds", "recordings", "backups"}
	for _, dir := range dirs {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(root, dir))
			if err != nil {
				t.Fatalf("expected directory %s: %v", dir, err)
			}
			if !info.IsDir() || info.Mode().Perm() != 0o750 {
				t.Fatalf("expected %s mode 0750 directory, got dir=%v mode=%o", dir, info.IsDir(), info.Mode().Perm())
			}
		})
	}

	integrationPath := filepath.Join(root, "integration.env")
	content, err := os.ReadFile(integrationPath)
	if err != nil {
		t.Fatalf("expected integration.env: %v", err)
	}
	if !strings.Contains(string(content), "ASTERISK_AMI_PASSWORD=secret") {
		t.Fatalf("integration.env should render shared credentials, got:\n%s", content)
	}
	info, err := os.Stat(integrationPath)
	if err != nil {
		t.Fatalf("expected integration.env stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected integration.env mode 0600, got %o", info.Mode().Perm())
	}
}
