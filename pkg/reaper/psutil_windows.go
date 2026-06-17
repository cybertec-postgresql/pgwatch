package reaper

import (
	"os"
	"syscall"
)

// GetPathUnderlyingDeviceID returns the volume serial number for the volume
// that hosts the given path. This is the Windows equivalent of the Unix
// device ID (Stat_t.Dev) and uniquely identifies the underlying disk volume.
func GetPathUnderlyingDeviceID(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var info syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return 0, err
	}
	return uint64(info.VolumeSerialNumber), nil
}
