package guard

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func openLock(directory string) (*os.File, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid security state directory")
	}
	f, err := os.OpenFile(filepath.Join(directory, "state.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlapped windows.Overlapped
	if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		f.Close()
		return nil, errors.New("security state is in use; stop the API before offline recovery")
	}
	return f, nil
}
func releaseLock(f *os.File) {
	var o windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &o)
	_ = f.Close()
}
