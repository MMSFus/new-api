//go:build !windows

package sessionrecorder

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openNoFollow(name string) (*os.File, error) {
	fd, err := unix.Open(name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return os.NewFile(uintptr(fd), name), nil
}

func storageUsage(path string) (int, int, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	disk, inode := 0, 0
	used := uint64(st.Blocks) - uint64(st.Bfree)
	if avail := used + uint64(st.Bavail); avail > 0 {
		disk = int((used*100 + avail - 1) / avail)
	}
	if st.Files > 0 {
		inode = int(((uint64(st.Files) - uint64(st.Ffree)) * 100) / uint64(st.Files))
	}
	return disk, inode, nil
}

// fileIdentity returns the device and inode of an open file.
func fileIdentity(info os.FileInfo) (uint64, uint64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev), uint64(st.Ino)
	}
	return 0, 0
}

type processLock struct{ file *os.File }

// AcquireProcessLock takes an exclusive, non-blocking flock on name.
func AcquireProcessLock(name string) (*processLock, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("recorder lock already held: %s", name)
	}
	return &processLock{file: f}, nil
}

func (l *processLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}
