package asterisk

import "context"

type PackageManagerKind string

const (
	PackageManagerUnknown PackageManagerKind = "unknown"
	PackageManagerAPT     PackageManagerKind = "apt"
	PackageManagerDNF     PackageManagerKind = "dnf"
	PackageManagerYUM     PackageManagerKind = "yum"
	PackageManagerPacman  PackageManagerKind = "pacman"
)

const (
	AsteriskServiceName  = "asterisk"
	AsteriskPackageName  = "asterisk"
	ManagerConfigPath    = "/etc/asterisk/manager.conf"
	ExtensionsConfigPath = "/etc/asterisk/extensions.conf"
	DefaultConfigRoot    = "/opt/alice-config/asterisk"
	DefaultAMIHost       = "127.0.0.1"
	DefaultAMIPort       = 5038
)

type HostDetector interface {
	Detect(context.Context) (HostState, error)
}

type PackageManager interface {
	IsInstalled(context.Context, string) (bool, error)
	Install(context.Context, string) error
	Remove(context.Context, string) error
}

type ServiceManager interface {
	IsEnabled(context.Context, string) (bool, error)
	IsActive(context.Context, string) (bool, error)
	Enable(context.Context, string) error
	Restart(context.Context, string) error
	Disable(context.Context, string) error
	Stop(context.Context, string) error
}

type ManagedConfigStore interface {
	ReadConfig(path string) (content string, exists bool, err error)
	WriteConfig(path string, content string) error
	DeleteConfig(path string) error
}

type SharedResourceStore interface {
	SnapshotBundle(root string) (ResourceBundleSnapshot, error)
	CreateBundle(root string, contract AMIContract) error
	RestoreBundle(snapshot ResourceBundleSnapshot) error
}

type AMIProbe interface {
	VerifyAMI(ctx context.Context, host string, port int, username string, password string) error
}

type Dependencies struct {
	Detector  HostDetector
	Packages  PackageManager
	Services  ServiceManager
	Configs   ManagedConfigStore
	Resources SharedResourceStore
	Probe     AMIProbe
}

type AMIContract struct {
	Enabled   bool
	Host      string
	Port      int
	Username  string
	Password  string
	ConfigDir string
}

type Options struct {
	Enabled    bool
	ConfigRoot string
	AMI        AMIContract
}

type Result struct {
	Installed   bool
	AMIEndpoint string
	Resources   string
}
