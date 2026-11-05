//go:build unix

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutePreservesProcess(t *testing.T) {
	cwd := t.TempDir()
	writeRoot(t, filepath.Join(cwd, "dev"), "1.15.0")
	script := `#!/bin/sh
pwd -P
printf '%s\n' "${@}"
printf '%s\n' "${TFWRAPPER_TEST_VALUE}"
cat
printf 'diagnostic\n' >&2
exit 2
`
	command := processCommand(t, cwd, script, "-chdir=dev", "plan", "-var=label=a b", "-json")
	command.Env = append(command.Env, "TFWRAPPER_TEST_VALUE=inherited")
	command.Stdin = strings.NewReader("stdin content\n")
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	assert.Equal(t, 2, exit.ExitCode())
	physical, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	assert.Equal(t, physical+"\n-chdir=dev\nplan\n-var=label=a b\n-json\ninherited\nstdin content\n",
		string(output))
	assert.Equal(t, "diagnostic\n", stderr.String())
}

func TestExecutePreservesSignal(t *testing.T) {
	cwd := t.TempDir()
	writeRoot(t, cwd, "1.15.0")
	command := processCommand(t, cwd, "#!/bin/sh\nkill -TERM \"${$}\"\n", "plan")
	err := command.Run()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	status, ok := exit.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.Equal(t, syscall.SIGTERM, status.Signal())
}

func TestCommandProcess(t *testing.T) {
	if os.Getenv("TFWRAPPER_TEST_PROCESS") != "1" {
		return
	}
	command := newCommand(func(_ context.Context, constraints version.Constraints) (string, error) {
		if constraints.String() != "1.15.0" {
			return "", fmt.Errorf("unexpected version constraints: %s", constraints)
		}
		return os.Getenv("TFWRAPPER_TEST_EXECUTABLE"), nil
	}, execute)
	for index, arg := range os.Args {
		if arg == "--" {
			command.SetArgs(os.Args[index+1:])
			break
		}
	}
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func processCommand(t *testing.T, cwd, script string, args ...string) *exec.Cmd {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terraform")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	command := exec.CommandContext(t.Context(), os.Args[0],
		append([]string{"-test.run=^TestCommandProcess$", "--"}, args...)...)
	command.Dir = cwd
	command.Env = append(os.Environ(),
		"TFWRAPPER_TEST_PROCESS=1", "TFWRAPPER_TEST_EXECUTABLE="+path)
	return command
}
