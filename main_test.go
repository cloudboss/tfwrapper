package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandPassThrough(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	original, err := os.Getwd()
	require.NoError(t, err)
	writeRoot(t, filepath.Join(cwd, "dev"), ">= 1.9.0, < 2.0.0")
	args := []string{"-chdir=dev", "plan", "-var=label=a b", "-json", "--", "argument"}
	installed := false
	ran := false
	command := newCommand(func(ctx context.Context, constraints version.Constraints) (string, error) {
		installed = true
		assert.Equal(t, ">= 1.9.0, < 2.0.0", constraints.String())
		return "/cache/terraform", nil
	}, func(path string, received []string) error {
		ran = true
		assert.Equal(t, "/cache/terraform", path)
		assert.Equal(t, args, received)
		actual, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, original, actual)
		return nil
	})
	command.SetArgs(args)
	require.NoError(t, command.Execute())
	assert.True(t, installed)
	assert.True(t, ran)
}

func TestCommandRejectsInvalidRoot(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"-h"}, {"plan"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			t.Chdir(t.TempDir())
			command := newCommand(func(context.Context, version.Constraints) (string, error) {
				t.Fatal("download started before root validation")
				return "", nil
			}, func(string, []string) error {
				t.Fatal("Terraform started before root validation")
				return nil
			})
			command.SetArgs(args)
			err := command.Execute()
			require.ErrorContains(t, err, "must run from a root module or define -chdir")
			assert.ErrorContains(t, err, "terraform block")
		})
	}
}

func TestCommandSpecialArgumentsPassThrough(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"-h"}, {"help"}, {"--version"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeRoot(t, dir, "1.15.0")
			installed := false
			ran := false
			command := newCommand(func(context.Context, version.Constraints) (string, error) {
				installed = true
				return "/cache/terraform", nil
			}, func(path string, received []string) error {
				ran = true
				assert.Equal(t, "/cache/terraform", path)
				assert.Equal(t, args, received)
				return nil
			})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(args)
			require.NoError(t, command.Execute())
			assert.True(t, installed)
			assert.True(t, ran)
			assert.Empty(t, output.String())
		})
	}
}

func TestCommandDownloadFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeRoot(t, dir, "1.15.0")
	failure := errors.New("download failed")
	command := newCommand(func(context.Context, version.Constraints) (string, error) {
		return "", failure
	}, func(string, []string) error {
		t.Fatal("Terraform started after download failure")
		return nil
	})
	command.SetArgs([]string{"apply"})
	require.ErrorIs(t, command.Execute(), failure)
}

func TestCommandCancellation(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeRoot(t, dir, "1.15.0")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	command := newCommand(func(ctx context.Context, _ version.Constraints) (string, error) {
		return "", ctx.Err()
	}, nil)
	command.SetArgs([]string{"plan"})
	require.ErrorIs(t, command.ExecuteContext(ctx), context.Canceled)
}

func writeRoot(t *testing.T, dir, constraint string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	contents := fmt.Sprintf("terraform { required_version = %q }\n", constraint)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(contents), 0o644))
}
