//go:build linux || darwin

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readHiddenPasswordUnix(in *os.File, out io.Writer, readRequest, writeRequest uint) ([]byte, error) {
	fd := int(in.Fd())
	termios, err := unix.IoctlGetTermios(fd, readRequest)
	if err != nil {
		return nil, errors.New("stdin must be an interactive terminal; use docker run -it")
	}
	hidden := *termios
	hidden.Lflag &^= unix.ECHO
	hidden.Lflag |= unix.ICANON | unix.ISIG
	hidden.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, writeRequest, &hidden); err != nil {
		return nil, fmt.Errorf("disable terminal echo: %w", err)
	}
	defer unix.IoctlSetTermios(fd, writeRequest, termios)

	var password bytes.Buffer
	buffer := make([]byte, 4096)
	for password.Len() <= 4096 {
		count, err := unix.Read(fd, buffer)
		if count > 0 {
			password.Write(buffer[:count])
			if bytes.ContainsAny(buffer[:count], "\r\n") {
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("read password: %w", err)
		}
	}
	fmt.Fprintln(out)
	if password.Len() > 4096 {
		return nil, errors.New("password is too long")
	}
	return bytes.TrimRight(password.Bytes(), "\r\n"), nil
}
