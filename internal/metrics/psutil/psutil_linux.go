package psutil

import (
	"os"
	"syscall"
)

func GetPathUnderlyingDeviceID(path string) (uint64, error) {
	fp, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	fi, err := fp.Stat()
	if err != nil {
		return 0, err
	}
	stat := fi.Sys().(*syscall.Stat_t)
	return stat.Dev, nil
}
