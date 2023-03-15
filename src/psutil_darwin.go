package main

import "errors"

var ErrNotImplemented = errors.New("not implemented")

func getPathUnderlyingDeviceID(path string) (uint64, error) {
	return 0, ErrNotImplemented
}
