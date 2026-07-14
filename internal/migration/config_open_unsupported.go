//go:build !linux

package migration

import "errors"

var errSecureConfigOpenUnsupported = errors.New("secure config open is unsupported on this platform")

type unsupportedConfigOpener struct{}

func newConfigOpener() ConfigOpener { return unsupportedConfigOpener{} }
func (unsupportedConfigOpener) Open(string) (ConfigFile, error) {
	return nil, errSecureConfigOpenUnsupported
}
