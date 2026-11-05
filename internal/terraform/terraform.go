package terraform

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudboss/cachedeps"
	"github.com/hashicorp/go-version"
)

type Installer struct {
	cache   *cachedeps.Cache
	client  *http.Client
	baseURL string
}

func New(output io.Writer) *Installer {
	client := &http.Client{Timeout: 5 * time.Minute}
	cache := cachedeps.New(filepath.Join("tfwrapper", runtime.GOOS+"_"+runtime.GOARCH))
	cache.Output = output
	cache.Client = client
	return &Installer{
		cache: cache, client: client,
		baseURL: "https://releases.hashicorp.com/terraform",
	}
}

func (i *Installer) Ensure(ctx context.Context, constraints version.Constraints) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(constraints) == 0 {
		return "", fmt.Errorf("terraform.required_version must not be empty")
	}
	selected, err := i.resolve(ctx, constraints)
	if err != nil {
		return "", err
	}
	platform := cachedeps.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	manifest, err := i.cache.EnsureContext(ctx, cachedeps.Dependency{
		Name: "terraform-checksums", Version: selected, Format: cachedeps.Binary,
		URLs: map[cachedeps.Platform]string{
			platform: i.baseURL + "/" + selected + "/terraform_" + selected + "_SHA256SUMS",
		},
	})
	if err != nil {
		return "", fmt.Errorf("get Terraform %s checksums: %w", selected, err)
	}
	if err := os.Chmod(manifest, 0o644); err != nil {
		return "", fmt.Errorf("set Terraform checksum file permissions: %w", err)
	}
	digest, err := checksum(manifest, selected)
	if err != nil {
		return "", err
	}
	dependency := i.dependency(selected)
	dependency.SHA256 = map[cachedeps.Platform]string{platform: digest}
	return i.cache.EnsureContext(ctx, dependency)
}

func minimum(constraints version.Constraints, releases []string) (string, error) {
	_, exact := lowerBound(constraints)
	var selected *version.Version
	for _, release := range releases {
		candidate, err := version.NewVersion(release)
		if err != nil || !constraints.Check(candidate) {
			continue
		}
		if candidate.Prerelease() != "" && !exact {
			continue
		}
		if selected == nil || candidate.LessThan(selected) {
			selected = candidate
		}
	}
	if selected == nil {
		return "", fmt.Errorf("no published Terraform version satisfies required_version %q", constraints)
	}
	return selected.String(), nil
}

func (i *Installer) resolve(ctx context.Context, constraints version.Constraints) (string, error) {
	bound, exact := lowerBound(constraints)
	if bound != nil && constraints.Check(bound) {
		if exact {
			return bound.String(), nil
		}
		path, err := i.dependency(bound.String()).Path(i.cache)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return bound.String(), nil
		}
	} else if exact {
		return "", fmt.Errorf("no published Terraform version satisfies required_version %q", constraints)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.baseURL+"/index.json", nil)
	if err != nil {
		return "", fmt.Errorf("read Terraform release index: %w", err)
	}
	response, err := i.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("read Terraform release index: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("read Terraform release index: HTTP %d", response.StatusCode)
	}
	var index struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&index); err != nil {
		return "", fmt.Errorf("read Terraform release index: %w", err)
	}
	releases := make([]string, 0, len(index.Versions))
	for release := range index.Versions {
		releases = append(releases, release)
	}
	return minimum(constraints, releases)
}

func lowerBound(constraints version.Constraints) (*version.Version, bool) {
	var bound *version.Version
	for _, constraint := range constraints {
		raw := strings.TrimSpace(constraint.String())
		text := strings.TrimLeft(raw, "<>=!~ \t")
		operator := strings.TrimSpace(strings.TrimSuffix(raw, text))
		candidate, err := version.NewVersion(text)
		if err != nil {
			continue
		}
		switch operator {
		case "", "=":
			return candidate, true
		case ">=", "~>":
			if bound == nil || candidate.GreaterThan(bound) {
				bound = candidate
			}
		}
	}
	return bound, false
}

func (i *Installer) dependency(selected string) cachedeps.Dependency {
	platform := cachedeps.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	name := "terraform"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return cachedeps.Dependency{
		Name: "terraform", Version: selected, Format: cachedeps.Zip, Subpath: name,
		URLs: map[cachedeps.Platform]string{
			platform: i.baseURL + "/" + selected + "/" + archiveName(selected),
		},
	}
}

func checksum(path, selected string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read Terraform checksums: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != archiveName(selected) {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return "", fmt.Errorf("invalid SHA256 for Terraform %s", selected)
		}
		return strings.ToLower(fields[0]), nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read Terraform checksums: %w", err)
	}
	return "", fmt.Errorf(
		"terraform %s has no release for %s/%s", selected, runtime.GOOS, runtime.GOARCH,
	)
}

func archiveName(selected string) string {
	return fmt.Sprintf("terraform_%s_%s_%s.zip", selected, runtime.GOOS, runtime.GOARCH)
}
