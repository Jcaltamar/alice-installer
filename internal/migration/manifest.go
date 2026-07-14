package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

var ErrPublication = errors.New("backup publication failed")

type PublicationRequest struct {
	Directory string
	DumpPath  string
	Container ContainerIdentity
	Config    ResolvedConfig
	Now       time.Time

	dumpOwnership operationCreatedPath
	fault         string // test-only fault injection; no production caller can set it outside this package.
}

type BackupPublication struct {
	DumpPath     string
	ManifestPath string
	SHA256       string
	Size         int64
}

// operationCreatedPath is an unforgeable-to-callers capability: only this
// package can attach it to a publication request after creating staging.
type operationCreatedPath struct{ path string }

func ownedPublicationRequest(request PublicationRequest, dumpPath string) PublicationRequest {
	request.dumpOwnership = operationCreatedPath{path: dumpPath}
	return request
}

type backupManifest struct {
	SchemaVersion int    `json:"schema_version"`
	CreatedAt     string `json:"created_at"`
	ContainerID   string `json:"container_id"`
	Image         string `json:"image"`
	ImageDigest   string `json:"image_digest,omitempty"`
	Environment   string `json:"environment"`
	Database      string `json:"database"`
	Username      string `json:"username"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Format        string `json:"format"`
	Size          int64  `json:"byte_size"`
	SHA256        string `json:"sha256"`
	DumpClient    string `json:"dump_client"`
	RestoreClient string `json:"restore_client"`
	Validation    string `json:"validation"`
	ToolSchema    int    `json:"tool_schema_version"`
}

// PublishBackupPair rejects requests without the package-private ownership
// capability. It never removes a caller-supplied path.
func PublishBackupPair(ctx context.Context, request PublicationRequest) (BackupPublication, error) {
	return publishBackupPair(ctx, request)
}

// publishBackupPair turns an operation-owned validated staging dump into a
// durable pair. It never replaces a name and removes only paths successfully
// created by this operation.
func publishBackupPair(ctx context.Context, request PublicationRequest) (BackupPublication, error) {
	if !validPublicationRequest(request) || request.dumpOwnership.path != request.DumpPath {
		return BackupPublication{}, ErrPublication
	}
	dumpStagedCreated := true // validated by the package-private ownership capability above.
	var dumpFinal, manifestFinal, manifestStaged string
	dumpFinalCreated, manifestStagedCreated, manifestFinalCreated := false, false, false
	defer func() {
		if dumpStagedCreated {
			cleanupPath(request.DumpPath)
		}
		if manifestStagedCreated {
			cleanupPath(manifestStaged)
		}
		if dumpFinalCreated {
			cleanupPath(dumpFinal)
		}
		if manifestFinalCreated {
			cleanupPath(manifestFinal)
		}
	}()
	if ctx.Err() != nil {
		return BackupPublication{}, ErrPublication
	}
	sum, size, err := hashFile(request.DumpPath)
	if err != nil || size == 0 || ctx.Err() != nil {
		return BackupPublication{}, ErrPublication
	}
	base := "alice-backup-" + sum
	dumpFinal = filepath.Join(request.Directory, base+".dump")
	manifestFinal = filepath.Join(request.Directory, base+".manifest.json")
	manifestStaged = filepath.Join(request.Directory, "."+base+".manifest.part")

	now := request.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	encoded, err := json.Marshal(backupManifest{
		SchemaVersion: 1, CreatedAt: now.Format(time.RFC3339Nano), ContainerID: request.Container.ID,
		Image: string(request.Container.Image), ImageDigest: request.Container.Digest,
		Environment: string(request.Config.Environment), Database: request.Config.Database, Username: request.Config.Username,
		Host: request.Config.Host, Port: request.Config.Port, Format: "postgresql-custom", Size: size, SHA256: sum,
		DumpClient: "postgresql-11", RestoreClient: "postgresql-11", Validation: "validated", ToolSchema: 1,
	})
	if err != nil || writeProtectedFile(manifestStaged, append(encoded, '\n')) != nil || ctx.Err() != nil {
		return BackupPublication{}, ErrPublication
	}
	manifestStagedCreated = true
	if request.fault == "dump-rename" || renameNoReplace(request.DumpPath, dumpFinal) != nil {
		return BackupPublication{}, ErrPublication
	}
	dumpStagedCreated = false
	dumpFinalCreated = true
	if request.fault == "manifest-rename" || renameNoReplace(manifestStaged, manifestFinal) != nil {
		return BackupPublication{}, ErrPublication
	}
	manifestStagedCreated = false
	manifestFinalCreated = true
	if ctx.Err() != nil || request.fault == "directory-sync" || syncDirectory(request.Directory) != nil {
		return BackupPublication{}, ErrPublication
	}
	dumpFinalCreated, manifestFinalCreated = false, false
	return BackupPublication{DumpPath: dumpFinal, ManifestPath: manifestFinal, SHA256: sum, Size: size}, nil
}

func validPublicationRequest(r PublicationRequest) bool {
	if r.Directory == "" || !safeStagedDump(r.DumpPath) || r.Container.Image != PostgreSQL11Image || !fullContainerID.MatchString(r.Container.ID) || !validConfig(r.Config) {
		return false
	}
	dir, err := filepath.Abs(r.Directory)
	return err == nil && dir == filepath.Dir(r.DumpPath)
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func writeProtectedFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanupPath(path)
	}
	return err
}

func renameNoReplace(oldPath, newPath string) error {
	return unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func cleanupPath(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func (m backupManifest) String() string {
	return fmt.Sprintf("backup manifest schema=%d", m.SchemaVersion)
}
