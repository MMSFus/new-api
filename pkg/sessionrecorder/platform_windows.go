//go:build windows

package sessionrecorder

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func openNoFollow(name string) (*os.File, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, &os.PathError{Op: "open", Path: name, Err: errors.New("symlink not allowed")}
	}
	return os.Open(name)
}

func storageUsage(path string) (int, int, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeAvail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &total, &free); err != nil {
		return 0, 0, err
	}
	if total == 0 {
		return 0, 0, nil
	}
	return int(((total - free) * 100) / total), 0, nil
}

func fileIdentity(os.FileInfo) (uint64, uint64) { return 0, 0 }

type processLock struct{ file *os.File }

// AcquireProcessLock takes an exclusive, non-blocking LockFileEx on name.
func AcquireProcessLock(name string) (*processLock, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	ol := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, fmt.Errorf("recorder lock already held: %s", name)
	}
	return &processLock{file: f}, nil
}

func (l *processLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, ol)
	_ = l.file.Close()
	l.file = nil
}
