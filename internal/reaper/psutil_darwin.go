package reaper

import "errors"

var ErrNotImplemented = errors.New("not implemented")

func GetPathUnderlyingDeviceID(path string) (uint64, error) {
	return 0, ErrNotImplemented
}
