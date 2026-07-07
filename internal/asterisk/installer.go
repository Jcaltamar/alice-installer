package asterisk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Installer struct {
	deps Dependencies
}

func NewInstaller(deps Dependencies) *Installer {
	return &Installer{deps: deps}
}

func (i *Installer) Install(ctx context.Context, opts Options) (Result, error) {
	if !opts.Enabled {
		return Result{}, nil
	}
	if err := i.validateDependencies(); err != nil {
		return Result{}, err
	}
	contract := NormalizeContract(opts.AMI, opts.ConfigRoot)
	if contract.Username == "" || contract.Password == "" {
		return Result{}, optionalSetupError("credential validation", errors.New("AMI username and password are required"))
	}
	if contract.Host != DefaultAMIHost {
		return Result{}, optionalSetupError("ami configuration", fmt.Errorf("AMI must bind to %s, got %s", DefaultAMIHost, contract.Host))
	}
	if contract.Port != DefaultAMIPort {
		return Result{}, optionalSetupError("ami configuration", fmt.Errorf("AMI must listen on port %d, got %d", DefaultAMIPort, contract.Port))
	}

	host, err := i.deps.Detector.Detect(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := host.Validate(); err != nil {
		return Result{}, err
	}

	snapshot, err := i.snapshot(ctx, contract.ConfigDir)
	if err != nil {
		return Result{}, optionalSetupError("host snapshot", err)
	}
	packageInstalledByInstaller := false

	if !snapshot.packagePreInstalled {
		if err := i.deps.Packages.Install(ctx, AsteriskPackageName); err != nil {
			return Result{}, optionalSetupError("package install", err)
		}
		packageInstalledByInstaller = true
	}

	if err := i.writeManagedConfigs(contract); err != nil {
		i.rollback(ctx, snapshot, packageInstalledByInstaller)
		return Result{}, optionalSetupError("configuration", err)
	}
	if err := i.deps.Resources.CreateBundle(contract.ConfigDir, contract); err != nil {
		i.rollback(ctx, snapshot, packageInstalledByInstaller)
		return Result{}, optionalSetupError("shared resources", err)
	}
	if err := i.deps.Services.Enable(ctx, AsteriskServiceName); err != nil {
		i.rollback(ctx, snapshot, packageInstalledByInstaller)
		return Result{}, optionalSetupError("service enable", err)
	}
	if err := i.deps.Services.Restart(ctx, AsteriskServiceName); err != nil {
		i.rollback(ctx, snapshot, packageInstalledByInstaller)
		return Result{}, optionalSetupError("service restart", err)
	}
	if err := i.deps.Probe.VerifyAMI(ctx, contract.Host, contract.Port, contract.Username, contract.Password); err != nil {
		i.rollback(ctx, snapshot, packageInstalledByInstaller)
		return Result{}, optionalSetupError("ami verification", err)
	}

	return Result{Installed: true, AMIEndpoint: fmt.Sprintf("%s:%d", contract.Host, contract.Port), Resources: contract.ConfigDir}, nil
}

func (i *Installer) validateDependencies() error {
	if i.deps.Detector == nil || i.deps.Packages == nil || i.deps.Services == nil || i.deps.Configs == nil || i.deps.Resources == nil || i.deps.Probe == nil {
		return errors.New("asterisk installer dependencies are incomplete")
	}
	return nil
}

func (i *Installer) snapshot(ctx context.Context, resourceRoot string) (installSnapshot, error) {
	installed, err := i.deps.Packages.IsInstalled(ctx, AsteriskPackageName)
	if err != nil {
		return installSnapshot{}, err
	}
	enabled, err := i.deps.Services.IsEnabled(ctx, AsteriskServiceName)
	if err != nil {
		return installSnapshot{}, err
	}
	active, err := i.deps.Services.IsActive(ctx, AsteriskServiceName)
	if err != nil {
		return installSnapshot{}, err
	}

	snapshot := installSnapshot{
		packagePreInstalled: installed,
		servicePreEnabled:   enabled,
		servicePreActive:    active,
		configs:             make(map[string]configSnapshot),
	}
	for _, path := range []string{ManagerConfigPath, ExtensionsConfigPath} {
		content, exists, err := i.deps.Configs.ReadConfig(path)
		if err != nil {
			return installSnapshot{}, err
		}
		snapshot.configs[path] = configSnapshot{content: content, exists: exists}
	}
	resources, err := i.deps.Resources.SnapshotBundle(resourceRoot)
	if err != nil {
		return installSnapshot{}, err
	}
	snapshot.resources = resources
	return snapshot, nil
}

func (i *Installer) writeManagedConfigs(contract AMIContract) error {
	manager, _, err := i.deps.Configs.ReadConfig(ManagerConfigPath)
	if err != nil {
		return err
	}
	if err := validateExistingManagerConfig(manager); err != nil {
		return err
	}
	manager = ReplaceManagedSection(manager, "ami", RenderManagerAMISection(contract))
	if err := i.deps.Configs.WriteConfig(ManagerConfigPath, manager); err != nil {
		return err
	}

	extensions, _, err := i.deps.Configs.ReadConfig(ExtensionsConfigPath)
	if err != nil {
		return err
	}
	extensions = ReplaceManagedSection(extensions, "extensions", RenderExtensionsSection())
	return i.deps.Configs.WriteConfig(ExtensionsConfigPath, extensions)
}

func validateExistingManagerConfig(content string) error {
	begin := fmt.Sprintf(managedBeginFormat, "ami")
	end := fmt.Sprintf(managedEndFormat, "ami")
	inManaged := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == begin {
			inManaged = true
			continue
		}
		if line == end {
			inManaged = false
			continue
		}
		if inManaged || line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "bindaddr") {
			continue
		}
		if strings.TrimSpace(value) != DefaultAMIHost {
			return fmt.Errorf("existing AMI bindaddr must be %s before alice-installer can manage Asterisk safely", DefaultAMIHost)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (i *Installer) rollback(ctx context.Context, snapshot installSnapshot, packageInstalledByInstaller bool) {
	_ = i.deps.Resources.RestoreBundle(snapshot.resources)
	for path, saved := range snapshot.configs {
		if saved.exists {
			_ = i.deps.Configs.WriteConfig(path, saved.content)
			continue
		}
		_ = i.deps.Configs.DeleteConfig(path)
	}
	if !snapshot.servicePreActive {
		_ = i.deps.Services.Stop(ctx, AsteriskServiceName)
	}
	if !snapshot.servicePreEnabled {
		_ = i.deps.Services.Disable(ctx, AsteriskServiceName)
	}
	if packageInstalledByInstaller && !snapshot.packagePreInstalled {
		_ = i.deps.Packages.Remove(ctx, AsteriskPackageName)
	}
}

type FileConfigStore struct{}

func (FileConfigStore) ReadConfig(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(content), true, nil
}

func (FileConfigStore) WriteConfig(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o640)
}

func (FileConfigStore) DeleteConfig(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

type FileResourceStore struct{}

func (FileResourceStore) SnapshotBundle(root string) (ResourceBundleSnapshot, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResourceBundleSnapshot{root: root}, nil
		}
		return ResourceBundleSnapshot{}, err
	}

	snapshot := ResourceBundleSnapshot{root: root, exists: true}
	if !info.IsDir() {
		entry, err := snapshotResourceEntry(root, root, info)
		if err != nil {
			return ResourceBundleSnapshot{}, err
		}
		snapshot.entries = append(snapshot.entries, entry)
		return snapshot, nil
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry, err := snapshotResourceEntry(root, path, info)
		if err != nil {
			return err
		}
		snapshot.entries = append(snapshot.entries, entry)
		return nil
	})
	if err != nil {
		return ResourceBundleSnapshot{}, err
	}
	return snapshot, nil
}

func (FileResourceStore) CreateBundle(root string, contract AMIContract) error {
	contract = NormalizeContract(contract, root)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o750); err != nil {
		return err
	}
	for _, dir := range []string{"templates", "sounds", "recordings", "backups"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o750); err != nil {
			return err
		}
	}
	integrationPath := filepath.Join(root, "integration.env")
	if err := os.WriteFile(integrationPath, []byte(RenderIntegrationEnv(contract)), 0o600); err != nil {
		return err
	}
	return os.Chmod(integrationPath, 0o600)
}

func (FileResourceStore) RestoreBundle(snapshot ResourceBundleSnapshot) error {
	if snapshot.root == "" {
		return nil
	}
	if err := os.RemoveAll(snapshot.root); err != nil {
		return err
	}
	if !snapshot.exists {
		return nil
	}
	for _, entry := range snapshot.entries {
		path := filepath.Join(snapshot.root, entry.path)
		mode := fs.FileMode(entry.mode)
		switch {
		case entry.isSymlink:
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(entry.linkTarget, path); err != nil {
				return err
			}
		case entry.isDir:
			if err := os.MkdirAll(path, mode.Perm()); err != nil {
				return err
			}
			if err := os.Chmod(path, mode.Perm()); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(path, entry.content, mode.Perm()); err != nil {
				return err
			}
			if err := os.Chmod(path, mode.Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func snapshotResourceEntry(root string, path string, info fs.FileInfo) (resourceSnapshotEntry, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return resourceSnapshotEntry{}, err
	}
	entry := resourceSnapshotEntry{path: relative, mode: uint32(info.Mode()), isDir: info.IsDir()}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return resourceSnapshotEntry{}, err
		}
		entry.isSymlink = true
		entry.linkTarget = target
		return entry, nil
	}
	if !info.IsDir() {
		content, err := os.ReadFile(path)
		if err != nil {
			return resourceSnapshotEntry{}, err
		}
		entry.content = content
	}
	return entry, nil
}
