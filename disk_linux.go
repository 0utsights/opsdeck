//go:build linux

package main

import "syscall"

func readDisk() (uint64, uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs("/", &st) != nil {
		return 0, 0
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	return total - free, total
}
