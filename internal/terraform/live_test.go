package terraform

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveTerraform(t *testing.T) {
	if os.Getenv("TFWRAPPER_LIVE_TEST") != "1" {
		t.Skip("set TFWRAPPER_LIVE_TEST=1 to download and run a real Terraform release")
	}
	installer := New(os.Stderr)
	installer.cache.Subdir = filepath.Join(
		"unobin", "tfwrapper-smoke", runtime.GOOS+"_"+runtime.GOARCH,
	)
	constraints, err := version.NewConstraint("1.15.0")
	require.NoError(t, err)
	path, err := installer.Ensure(t.Context(), constraints)
	require.NoError(t, err)
	output, err := exec.CommandContext(t.Context(), path, "version", "-json").Output()
	require.NoError(t, err)
	var result struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal(output, &result))
	assert.Equal(t, "1.15.0", result.Version)
	installer.baseURL = "http://127.0.0.1:1"
	constraints, err = version.NewConstraint(">= 1.15.0, < 1.16.0")
	require.NoError(t, err)
	cached, err := installer.Ensure(t.Context(), constraints)
	require.NoError(t, err)
	assert.Equal(t, path, cached)
}
