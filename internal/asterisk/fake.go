package asterisk

import (
	"context"
	"errors"
)

type FakeEnvironment struct {
	Detector  *FakeDetector
	Packages  *FakePackageManager
	Services  *FakeServiceManager
	Configs   *FakeConfigStore
	Resources *FakeResourceStore
	Probe     *FakeAMIProbe
}

func NewFakeEnvironment() *FakeEnvironment {
	return &FakeEnvironment{
		Detector:  &FakeDetector{State: SupportedHost(PackageManagerAPT)},
		Packages:  &FakePackageManager{},
		Services:  &FakeServiceManager{},
		Configs:   &FakeConfigStore{Files: make(map[string]string)},
		Resources: &FakeResourceStore{},
		Probe:     &FakeAMIProbe{},
	}
}

func (f *FakeEnvironment) Dependencies() Dependencies {
	return Dependencies{Detector: f.Detector, Packages: f.Packages, Services: f.Services, Configs: f.Configs, Resources: f.Resources, Probe: f.Probe}
}

type FakeDetector struct {
	State HostState
	Err   error
}

func (f *FakeDetector) Detect(context.Context) (HostState, error) {
	if f.Err != nil {
		return HostState{}, f.Err
	}
	return f.State, nil
}

type FakePackageManager struct {
	PreInstalled      bool
	InstalledPackages []string
	RemoveCalls       int
	Err               error
}

func (f *FakePackageManager) IsInstalled(context.Context, string) (bool, error) {
	return f.PreInstalled || len(f.InstalledPackages) > 0, f.Err
}

func (f *FakePackageManager) Install(_ context.Context, name string) error {
	if f.Err != nil {
		return f.Err
	}
	f.InstalledPackages = append(f.InstalledPackages, name)
	return nil
}

func (f *FakePackageManager) Remove(context.Context, string) error {
	f.RemoveCalls++
	return f.Err
}

type FakeServiceManager struct {
	PreEnabled   bool
	PreActive    bool
	EnableCalls  int
	RestartCalls int
	DisableCalls int
	StopCalls    int
	Err          error
}

func (f *FakeServiceManager) IsEnabled(context.Context, string) (bool, error) {
	return f.PreEnabled, f.Err
}

func (f *FakeServiceManager) IsActive(context.Context, string) (bool, error) {
	return f.PreActive, f.Err
}

func (f *FakeServiceManager) Enable(context.Context, string) error {
	f.EnableCalls++
	return f.Err
}

func (f *FakeServiceManager) Restart(context.Context, string) error {
	f.RestartCalls++
	return f.Err
}

func (f *FakeServiceManager) Disable(context.Context, string) error {
	f.DisableCalls++
	return f.Err
}

func (f *FakeServiceManager) Stop(context.Context, string) error {
	f.StopCalls++
	return f.Err
}

type FakeConfigStore struct {
	Files map[string]string
	Err   error
}

func (f *FakeConfigStore) ReadConfig(path string) (string, bool, error) {
	if f.Err != nil {
		return "", false, f.Err
	}
	if f.Files == nil {
		f.Files = make(map[string]string)
	}
	content, ok := f.Files[path]
	return content, ok, nil
}

func (f *FakeConfigStore) WriteConfig(path string, content string) error {
	if f.Err != nil {
		return f.Err
	}
	if f.Files == nil {
		f.Files = make(map[string]string)
	}
	f.Files[path] = content
	return nil
}

func (f *FakeConfigStore) DeleteConfig(path string) error {
	if f.Err != nil {
		return f.Err
	}
	delete(f.Files, path)
	return nil
}

type FakeResourceStore struct {
	Root              string
	Contract          AMIContract
	RestoreCalls      int
	SnapshotRoot      string
	RestoredSnapshot  ResourceBundleSnapshot
	ExistingSnapshot  ResourceBundleSnapshot
	BundleExists      bool
	IntegrationEnv    string
	OriginalResources map[string]string
	Err               error
}

func (f *FakeResourceStore) SnapshotBundle(root string) (ResourceBundleSnapshot, error) {
	if f.Err != nil {
		return ResourceBundleSnapshot{}, f.Err
	}
	f.SnapshotRoot = root
	if f.ExistingSnapshot.root != "" || f.ExistingSnapshot.exists {
		return f.ExistingSnapshot, nil
	}
	return ResourceBundleSnapshot{root: root, exists: f.BundleExists}, nil
}

func (f *FakeResourceStore) CreateBundle(root string, contract AMIContract) error {
	if f.Err != nil {
		return f.Err
	}
	if root == "" {
		return errors.New("resource root is required")
	}
	f.Root = root
	f.Contract = contract
	f.BundleExists = true
	f.IntegrationEnv = RenderIntegrationEnv(NormalizeContract(contract, root))
	return nil
}

func (f *FakeResourceStore) RestoreBundle(snapshot ResourceBundleSnapshot) error {
	if f.Err != nil {
		return f.Err
	}
	f.RestoreCalls++
	f.RestoredSnapshot = snapshot
	f.BundleExists = snapshot.exists
	if !snapshot.exists {
		f.IntegrationEnv = ""
	}
	return nil
}

type FakeAMIProbe struct {
	Host     string
	Port     int
	Username string
	Password string
	Err      error
}

func (f *FakeAMIProbe) VerifyAMI(_ context.Context, host string, port int, username string, password string) error {
	f.Host = host
	f.Port = port
	f.Username = username
	f.Password = password
	return f.Err
}
