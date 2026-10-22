//go:build unix

package transfer

import "syscall"

// freeSpace returns the bytes free to this user on the disk holding dir.
func freeSpace(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
