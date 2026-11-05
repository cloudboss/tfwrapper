//go:build !unix

package main

import (
	"os"
	"os/exec"
)

func execute(path string, args []string) error {
	command := exec.Command(path, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}
