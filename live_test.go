package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveWorkflow(t *testing.T) {
	if os.Getenv("TFWRAPPER_LIVE_TEST") != "1" {
		t.Skip("set TFWRAPPER_LIVE_TEST=1 to build the wrapper and run Terraform")
	}
	binary := filepath.Join(t.TempDir(), "tfwrapper")
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binary, ".")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	project := t.TempDir()
	dir := filepath.Join(project, "dev")
	writeRoot(t, dir, ">= 1.15.0, < 1.16.0")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outputs.tf"),
		[]byte("output \"example\" { value = \"hello\" }\n"), 0o644))
	cache := t.TempDir()
	environment := append(os.Environ(),
		"HOME="+cache, "XDG_CACHE_HOME="+cache, "CHECKPOINT_DISABLE=1")
	command := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(t.Context(), binary, append([]string{"-chdir=dev"}, args...)...)
		cmd.Dir = project
		cmd.Env = environment
		return cmd
	}
	version := command("version", "-json")
	var progress bytes.Buffer
	version.Stderr = &progress
	output, err = version.Output()
	require.NoError(t, err, "%s", progress.String())
	var initial struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal(output, &initial))
	assert.Equal(t, "1.15.0", initial.Version)
	assert.Contains(t, progress.String(), "Downloading terraform")
	output, err = command("--help").Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "Usage: terraform")
	output, err = command("init", "-backend=false", "-input=false", "-no-color").CombinedOutput()
	require.NoError(t, err, "%s", output)
	plan := command("plan", "-detailed-exitcode", "-input=false", "-json")
	output, err = plan.Output()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 2, exit.ExitCode(), "%s\n%s", output, exit.Stderr)
	decoder := json.NewDecoder(bytes.NewReader(output))
	var eventTypes []string
	for {
		var event struct {
			Type string `json:"type"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		eventTypes = append(eventTypes, event.Type)
	}
	assert.Contains(t, eventTypes, "change_summary")
	environment = append(environment,
		"HTTPS_PROXY=http://127.0.0.1:1", "HTTP_PROXY=http://127.0.0.1:1")
	output, err = command("version", "-json").Output()
	require.NoError(t, err)
	var reported struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal(output, &reported))
	assert.Equal(t, "1.15.0", reported.Version)
}
