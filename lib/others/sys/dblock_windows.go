package sys

import (
	"os"
)

var (
	DbLockFileName string
	DbLockFileHndl *os.File
)

func LockDatabaseDir(DuodHomeDir string) {
	var e error
	os.MkdirAll(DuodHomeDir, 0770)
	DbLockFileName = DuodHomeDir+".lock"
	os.Remove(DbLockFileName)
	DbLockFileHndl, e = os.OpenFile(DbLockFileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
	if e != nil {
		println(e.Error())
		println("Could not lock the databse folder for writing. Another instance might be running.")
		println("Make sure you can delete and recreate file:", DbLockFileName)
		os.Exit(1)
	}
}

func UnlockDatabaseDir() {
	DbLockFileHndl.Close()
	os.Remove(DbLockFileName)
}
