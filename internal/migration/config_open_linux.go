//go:build linux

package migration

import (
	"os"

	"golang.org/x/sys/unix"
)

type linuxConfigOpener struct{}

func newConfigOpener() ConfigOpener { return linuxConfigOpener{} }

func (linuxConfigOpener) Open(path string) (ConfigFile, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
