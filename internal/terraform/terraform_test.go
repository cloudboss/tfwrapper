package terraform

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimum(t *testing.T) {
	releases := []string{"2.0.0", "1.10.0", "1.9.1", "1.9.0", "1.11.0-rc1", "invalid"}
	tests := []struct {
		constraint string
		want       string
	}{
		{constraint: "1.9.1", want: "1.9.1"},
		{constraint: ">= 1.9.0, < 2.0.0", want: "1.9.0"},
		{constraint: "> 1.9.0", want: "1.9.1"},
		{constraint: "~> 1.9.0", want: "1.9.0"},
		{constraint: ">= 1.9.0, != 1.9.0", want: "1.9.1"},
		{constraint: ">= 1.9.2", want: "1.10.0"},
		{constraint: ">= 1.9.0, >= 1.10.0", want: "1.10.0"},
		{constraint: ">= 1.10.0, <= 1.9.0"},
		{constraint: ">= 1.11.0, < 2.0.0"},
		{constraint: "1.11.0-rc1", want: "1.11.0-rc1"},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			constraints, err := version.NewConstraint(tt.constraint)
			require.NoError(t, err)
			selected, err := minimum(constraints, releases)
			if tt.want == "" {
				require.ErrorContains(t, err, "no published Terraform version")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, selected)
		})
	}
}

func TestEnsureDownloadsAndReuses(t *testing.T) {
	for _, constraint := range []string{"1.9.0", ">= 1.9.0, < 2.0.0"} {
		t.Run(constraint, func(t *testing.T) {
			installer := testInstaller(t)
			archive := testArchive(t, "selected Terraform")
			var indexRequests atomic.Int32
			var archiveRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/index.json":
					indexRequests.Add(1)
					_, _ = fmt.Fprint(w, `{"versions":{"1.10.0":{},"1.9.0":{}}}`)
				case r.URL.Path == "/1.9.0/terraform_1.9.0_SHA256SUMS":
					_, _ = fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(archive), archiveName("1.9.0"))
				case r.URL.Path == "/1.9.0/"+archiveName("1.9.0"):
					archiveRequests.Add(1)
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			installer.baseURL = server.URL
			constraints, err := version.NewConstraint(constraint)
			require.NoError(t, err)
			path, err := installer.Ensure(t.Context(), constraints)
			require.NoError(t, err)
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "selected Terraform", string(contents))
			assert.Contains(t, path, runtime.GOOS+"_"+runtime.GOARCH)
			cacheRoot, err := installer.cache.Root()
			require.NoError(t, err)
			manifest := filepath.Join(cacheRoot, "terraform-checksums-1.9.0")
			info, err := os.Stat(manifest)
			require.NoError(t, err)
			assert.Zero(t, info.Mode().Perm()&0o111)
			require.NoError(t, os.Chmod(manifest, 0o755))
			server.Close()
			cached, err := installer.Ensure(t.Context(), constraints)
			require.NoError(t, err)
			assert.Equal(t, path, cached)
			info, err = os.Stat(manifest)
			require.NoError(t, err)
			assert.Zero(t, info.Mode().Perm()&0o111)
			assert.Equal(t, int32(1), archiveRequests.Load())
			if constraint == "1.9.0" {
				assert.Zero(t, indexRequests.Load())
			} else {
				assert.Equal(t, int32(1), indexRequests.Load())
			}
		})
	}
}

func TestEnsureErrors(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		index      string
		manifest   string
		status     int
		failure    string
	}{
		{
			name: "unavailable index", constraint: "> 1.9.0", status: http.StatusBadGateway,
			failure: "HTTP 502",
		},
		{
			name: "malformed index", constraint: "> 1.9.0", index: `{`,
			failure: "release index",
		},
		{
			name: "no matching release", constraint: ">= 9.0.0", index: `{"versions":{"1.9.0":{}}}`,
			failure: "no published Terraform version",
		},
		{
			name: "unsupported platform", constraint: "1.9.0", manifest: "abc  another_platform.zip\n",
			failure: "no release for",
		},
		{
			name: "invalid checksum", constraint: "1.9.0",
			manifest: "invalid  " + archiveName("1.9.0") + "\n", failure: "invalid SHA256",
		},
		{
			name: "checksum mismatch", constraint: "1.9.0",
			manifest: strings.Repeat("0", 64) + "  " + archiveName("1.9.0") + "\n",
			failure:  "checksum mismatch",
		},
		{
			name: "unavailable exact version", constraint: "1.9.0", status: http.StatusNotFound,
			failure: "HTTP 404",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := testInstaller(t)
			archive := testArchive(t, "Terraform")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					return
				}
				switch {
				case r.URL.Path == "/index.json":
					_, _ = fmt.Fprint(w, tt.index)
				case strings.HasSuffix(r.URL.Path, "_SHA256SUMS"):
					_, _ = fmt.Fprint(w, tt.manifest)
				default:
					_, _ = w.Write(archive)
				}
			}))
			defer server.Close()
			installer.baseURL = server.URL
			constraints, err := version.NewConstraint(tt.constraint)
			require.NoError(t, err)
			_, err = installer.Ensure(t.Context(), constraints)
			require.ErrorContains(t, err, tt.failure)
		})
	}
}

func TestEnsureCancellation(t *testing.T) {
	installer := testInstaller(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	constraints, err := version.NewConstraint(">= 1.9.0")
	require.NoError(t, err)
	_, err = installer.Ensure(ctx, constraints)
	require.ErrorIs(t, err, context.Canceled)
}

func TestEnsureRequiresConstraints(t *testing.T) {
	_, err := testInstaller(t).Ensure(t.Context(), nil)
	require.ErrorContains(t, err, "required_version must not be empty")
}

func TestEnsureStrictBound(t *testing.T) {
	installer := testInstaller(t)
	archive := testArchive(t, "next published release")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"versions": map[string]any{"1.9.0": nil, "1.9.3": nil, "1.10.0": nil},
			})
		case r.URL.Path == "/1.9.3/terraform_1.9.3_SHA256SUMS":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(archive), archiveName("1.9.3"))
		case r.URL.Path == "/1.9.3/"+archiveName("1.9.3"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	installer.baseURL = server.URL
	constraints, err := version.NewConstraint("> 1.9.0")
	require.NoError(t, err)
	path, err := installer.Ensure(t.Context(), constraints)
	require.NoError(t, err)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "next published release", string(contents))
}

func testInstaller(t *testing.T) *Installer {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	return New(nil)
}

func testArchive(t *testing.T, contents string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	name := "terraform"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	require.NoError(t, err)
	_, err = entry.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}
