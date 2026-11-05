//go:build unix

package main

import (
	"os"
	"syscall"
)

func execute(path string, args []string) error {
	return syscall.Exec(path, append([]string{"terraform"}, args...), os.Environ())
}
