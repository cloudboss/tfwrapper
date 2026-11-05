# Changelog

## [Unreleased]

### Added

- Validate the root module and choose the lowest published Terraform version allowed by `required_version`.
- Download, verify, and cache Terraform executables with cachedeps.
- Pass Terraform arguments through, including `-chdir`, help flags, and version commands.
- Build release archives for Linux amd64 and arm64, macOS arm64, and Windows amd64, with SHA256 checksums and changelog release notes.
- Remove build outputs and read-only Go module caches with `make clean`.

[Unreleased]: https://github.com/cloudboss/tfwrapper/commits/main/
