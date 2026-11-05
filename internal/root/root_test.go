package root

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		constraints string
		parsed      []string
		failure     string
	}{
		{
			name: "exact version",
			files: map[string]string{
				"versions.tf": `terraform { required_version = "1.15.0" }`,
				"main.tf":     `module "example" { source = "./child" }`,
			},
			constraints: "1.15.0", parsed: []string{"main.tf", "versions.tf"},
		},
		{
			name: "multiple files and blocks",
			files: map[string]string{
				"a.tf": `terraform { required_version = ">= 1.9.0" }
terraform { required_version = "!= 1.10.0" }`,
				"b.tf.json": `{"terraform":{"required_version":"< 2.0.0"}}`,
			},
			constraints: ">= 1.9.0,!= 1.10.0,< 2.0.0", parsed: []string{"a.tf", "b.tf.json"},
		},
		{
			name: "overrides replace constraints by file",
			files: map[string]string{
				"base.tf": `terraform { required_version = "1.0.0" }`,
				"a_override.tf": `terraform { required_version = ">= 1.9.0" }
terraform { required_version = "< 2.0.0" }`,
				"z_override.tf.json": `{"terraform":{"required_version":"1.15.0"}}`,
			},
			constraints: "1.15.0",
			parsed:      []string{"base.tf", "a_override.tf", "z_override.tf.json"},
		},
		{
			name: "override without version preserves requirements",
			files: map[string]string{
				"main.tf": `terraform { required_version = "~> 1.15.0" }`,
				"override.tf": `terraform {
  backend "local" {}
}`,
			},
			constraints: "~> 1.15.0", parsed: []string{"main.tf", "override.tf"},
		},
		{
			name: "ignore hidden files and child modules",
			files: map[string]string{
				"main.tf":          `terraform { required_version = "1.15.0" }`,
				".hidden.tf":       "invalid syntax {",
				"child/main.tf":    "invalid syntax {",
				"terraform.tfvars": "invalid syntax {",
			},
			constraints: "1.15.0", parsed: []string{"main.tf"},
		},
		{name: "empty directory", failure: "terraform block"},
		{
			name:    "missing block",
			files:   map[string]string{"main.tf": `locals { name = "example" }`},
			failure: "terraform block",
		},
		{
			name:    "missing version",
			files:   map[string]string{"main.tf": `terraform {}`},
			failure: "required_version",
		},
		{
			name:    "empty version",
			files:   map[string]string{"main.tf": `terraform { required_version = "" }`},
			failure: "nonempty",
		},
		{
			name:    "invalid constraint",
			files:   map[string]string{"main.tf": `terraform { required_version = "latest" }`},
			failure: "main.tf:1",
		},
		{
			name:    "non-string version",
			files:   map[string]string{"main.tf": `terraform { required_version = 1.15 }`},
			failure: "string",
		},
		{
			name:    "null version",
			files:   map[string]string{"main.tf": `terraform { required_version = null }`},
			failure: "string",
		},
		{
			name:    "version from variable",
			files:   map[string]string{"main.tf": `terraform { required_version = var.version }`},
			failure: "Variables not allowed",
		},
		{
			name: "syntax error in unrelated configuration",
			files: map[string]string{
				"version.tf": `terraform { required_version = "1.15.0" }`,
				"main.tf":    `locals {`,
			},
			failure: "main.tf:1",
		},
		{
			name:    "invalid JSON",
			files:   map[string]string{"main.tf.json": `{"terraform":`},
			failure: "main.tf.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				path := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
			}
			module, err := Load(dir)
			if tt.failure != "" {
				require.ErrorContains(t, err, tt.failure)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, dir, module.Directory)
			parsed := make([]string, 0, len(module.Files))
			for path := range module.Files {
				parsed = append(parsed, filepath.Base(path))
			}
			assert.ElementsMatch(t, tt.parsed, parsed)
			assert.Equal(t, tt.constraints, module.Constraints.String())
		})
	}
}

func TestLoadMissingDirectory(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestDirectory(t *testing.T) {
	cwd := t.TempDir()
	tests := []struct {
		name    string
		args    []string
		want    string
		failure string
	}{
		{name: "current directory", args: []string{"plan"}, want: cwd},
		{
			name: "relative directory", args: []string{"-chdir=envs/dev", "plan"},
			want: filepath.Join(cwd, "envs/dev"),
		},
		{name: "absolute directory", args: []string{"-chdir=" + cwd, "version"}, want: cwd},
		{name: "empty directory", args: []string{"-chdir=", "plan"}, failure: "-chdir=path"},
		{name: "missing equals", args: []string{"-chdir", "dev", "plan"}, failure: "-chdir=path"},
		{
			name: "duplicate option", args: []string{"-chdir=a", "-chdir=b", "plan"},
			failure: "once",
		},
		{name: "command argument", args: []string{"plan", "-chdir=elsewhere"}, want: cwd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]string(nil), tt.args...)
			dir, err := Directory(cwd, tt.args)
			if tt.failure != "" {
				require.ErrorContains(t, err, tt.failure)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, dir)
			}
			assert.Equal(t, original, tt.args)
		})
	}
}
