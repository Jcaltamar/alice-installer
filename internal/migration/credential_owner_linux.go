//go:build linux

package migration

import (
	"os"
	"syscall"
)

func numericOwner(info os.FileInfo) (int, int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}

func ownedBy(info os.FileInfo, uid, gid int) bool {
	actualUID, actualGID, ok := numericOwner(info)
	return ok && actualUID == uid && actualGID == gid
}
