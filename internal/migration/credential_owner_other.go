//go:build !linux

package migration

import (
	"os"
	"os/user"
	"strconv"
)

func numericOwner(os.FileInfo) (int, int, bool) {
	current, err := user.Current()
	if err != nil {
		return 0, 0, false
	}
	uid, uidErr := strconv.Atoi(current.Uid)
	gid, gidErr := strconv.Atoi(current.Gid)
	return uid, gid, uidErr == nil && gidErr == nil
}

func ownedBy(info os.FileInfo, uid, gid int) bool {
	actualUID, actualGID, ok := numericOwner(info)
	return ok && actualUID == uid && actualGID == gid
}
