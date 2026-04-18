package docker

import "context"

// FakeDockerClient is a test double for DockerClient.
// Set the exported fields to control what each method returns.
type FakeDockerClient struct {
	ProbeErr    error
	InfoResult  Info
	InfoErr     error
	RuntimesMap map[string]bool
	VersionVal  Version
	VersionErr  error
}

// Probe returns ProbeErr.
func (f *FakeDockerClient) Probe(_ context.Context) error {
	return f.ProbeErr
}

// Info returns InfoResult, InfoErr.
func (f *FakeDockerClient) Info(_ context.Context) (Info, error) {
	return f.InfoResult, f.InfoErr
}

// Version returns VersionVal, VersionErr.
func (f *FakeDockerClient) Version(_ context.Context) (Version, error) {
	return f.VersionVal, f.VersionErr
}

// HasRuntime looks up name in RuntimesMap.
// Returns false when the map is nil or the key is absent.
func (f *FakeDockerClient) HasRuntime(_ context.Context, name string) (bool, error) {
	if f.RuntimesMap == nil {
		return false, nil
	}
	return f.RuntimesMap[name], nil
}
