//go:build darwin

package main

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readHiddenPassword(in *os.File, out io.Writer) ([]byte, error) {
	return readHiddenPasswordUnix(in, out, unix.TIOCGETA, unix.TIOCSETA)
}
