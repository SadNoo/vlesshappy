//go:build !linux && !darwin

package main

import (
	"errors"
	"io"
	"os"
)

func readHiddenPassword(_ *os.File, _ io.Writer) ([]byte, error) {
	return nil, errors.New("interactive setup is supported on Linux and macOS")
}
