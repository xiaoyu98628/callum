package run

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func checkSQLiteLinks(protection tool.ProtectedData) (retErr error) {
	if protection.Database == "" {
		return nil
	}
	for _, suffix := range []string{"", "-journal", "-wal", "-shm", ".lock"} {
		file, err := openTargetPath(protection.Database + suffix)
		if suffix != "" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var info windows.ByHandleFileInformation
		err = windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		if info.NumberOfLinks != 1 {
			return tool.ErrDenied
		}
	}
	return nil
}
