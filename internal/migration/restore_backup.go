package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcaltamar/alice-installer/internal/workspace"
)

var ErrRestoreBackupGate = errors.New("restore backup gate failed")

const defaultBackupRoot = "/opt/alice/backups/"

type BackupGate struct {
	Validator ArchiveValidator
}

func authoritativeBackupRoot() string { return defaultBackupRoot }

// Revalidate always confines production legacy artifacts to the fixed root.
func (g BackupGate) Revalidate(ctx context.Context, ref BackupRef) (ValidatedBackup, error) {
	return revalidateBackupInRoot(ctx, g.Validator, ref, authoritativeBackupRoot())
}

// revalidateBackupInRoot is a filesystem seam for unit tests. Production calls
// Revalidate, which supplies only the immutable authoritative root.
func revalidateBackupInRoot(ctx context.Context, validator ArchiveValidator, ref BackupRef, root string) (ValidatedBackup, error) {
	if ctx.Err() != nil || validator == nil || !validRestoreBackupInRoot(ref, root) {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	if err := validator.Validate(ctx, ref.DumpPath); err != nil || ctx.Err() != nil || !validRestoreBackupInRoot(ref, root) {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	return ValidatedBackup{ref: ref}, nil
}

type TargetRollbackBackupCreator struct {
	Validator   ArchiveValidator
	Stage       func(context.Context, workspace.TargetDatabaseConfig, string, string) (string, error)
	OperationID func() (string, error)
}

func (c TargetRollbackBackupCreator) CreateValidated(ctx context.Context, cfg workspace.TargetDatabaseConfig, destination string) (ValidatedBackup, error) {
	if ctx.Err() != nil || c.Validator == nil || c.Stage == nil || c.OperationID == nil || !filepath.IsAbs(destination) {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	operationID, err := c.OperationID()
	if err != nil || !validOperationID(operationID) {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	staged, err := c.Stage(ctx, cfg, destination, operationID)
	if err != nil || !safeTargetStaging(staged, destination, operationID) || ctx.Err() != nil {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	keep := false
	defer func() {
		if !keep {
			cleanupPath(staged)
		}
	}()
	if err := c.Validator.Validate(ctx, staged); err != nil || ctx.Err() != nil {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	ref, err := publishTargetRollback(staged, destination, operationID)
	if err != nil {
		return ValidatedBackup{}, ErrRestoreBackupGate
	}
	keep = true
	return ValidatedBackup{ref: ref, targetRollback: true}, nil
}

func ReplacementAllowed(legacy, target ValidatedBackup) bool {
	return !legacy.targetRollback && target.targetRollback && legacy.ref.DumpPath != target.ref.DumpPath && validRestoreBackup(legacy.ref) && validRestoreBackup(target.ref)
}

type restoreManifest struct {
	Format     string `json:"format"`
	Validation string `json:"validation"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"byte_size"`
}

func validRestoreBackupInRoot(ref BackupRef, root string) bool {
	return safeRestoreFileInRoot(ref.DumpPath, root) && safeRestoreFileInRoot(ref.ManifestPath, root) && validRestoreBackup(ref)
}

func safeRestoreFileInRoot(path, root string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if current != path && !info.IsDir() {
			return false
		}
		if current == path && (!info.Mode().IsRegular() || info.Size() <= 0) {
			return false
		}
	}
	return true
}

func validRestoreBackup(ref BackupRef) bool {
	if !safeRestoreFile(ref.DumpPath) || !safeRestoreFile(ref.ManifestPath) || ref.SHA256 == "" || ref.Size <= 0 {
		return false
	}
	sum, size, err := restoreHash(ref.DumpPath)
	if err != nil || sum != ref.SHA256 || size != ref.Size {
		return false
	}
	data, err := os.ReadFile(ref.ManifestPath)
	if err != nil {
		return false
	}
	var manifest restoreManifest
	return json.Unmarshal(data, &manifest) == nil && manifest.Format == "postgresql-custom" && manifest.Validation == "validated" && manifest.SHA256 == sum && manifest.Size == size
}
func safeRestoreFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && filepath.IsAbs(path) && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Size() > 0
}

func validOperationID(operationID string) bool {
	if len(operationID) == 0 || len(operationID) > 64 {
		return false
	}
	for index, character := range operationID {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || index > 0 && (character == '-' || character == '_') {
			continue
		}
		return false
	}
	return true
}

func safeTargetStaging(path, destination, operationID string) bool {
	info, err := os.Lstat(path)
	return validOperationID(operationID) && err == nil && info.Mode().Perm() == 0o600 && filepath.Dir(path) == destination && filepath.Base(path) == ".target-rollback-"+operationID+".part" && safeRestoreFile(path)
}

func publishTargetRollback(staged, destination, operationID string) (BackupRef, error) {
	sum, size, err := restoreHash(staged)
	if err != nil {
		return BackupRef{}, err
	}
	base := "alice-target-rollback-" + operationID + "-" + sum
	dump := filepath.Join(destination, base+".dump")
	manifest := dump + ".manifest.json"
	encoded, err := json.Marshal(restoreManifest{Format: "postgresql-custom", Validation: "validated", SHA256: sum, Size: size})
	if err != nil || writeProtectedFile(manifest, append(encoded, '\n')) != nil {
		return BackupRef{}, ErrRestoreBackupGate
	}
	manifestCreated := true
	defer func() {
		if manifestCreated {
			cleanupPath(manifest)
		}
	}()
	if renameNoReplace(staged, dump) != nil || syncDirectory(destination) != nil {
		cleanupPath(dump)
		return BackupRef{}, ErrRestoreBackupGate
	}
	manifestCreated = false
	return BackupRef{DumpPath: dump, ManifestPath: manifest, SHA256: sum, Size: size}, nil
}

func restoreHash(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), int64(len(data)), nil
}
