# Releasing tfwrapper

Branch pushes and pull requests run tests and lint. A pushed stable tag, `vMAJOR.MINOR.PATCH`, checks the changelog, runs the checks, builds the four archives, verifies them, and publishes a GitHub Release. The matching changelog section becomes the release notes.

## Local checks and builds

Install Docker, GNU Make, a POSIX shell, `zip`, `unzip`, and either `sha256sum` or `shasum`. The [Makefile](Makefile) pins the Go and golangci-lint images and uses them locally and in GitHub Actions. Keep the Go image version consistent with [go.mod](go.mod).

```sh
make test
make lint
make release VERSION=v0.1.0
make check-release-assets VERSION=v0.1.0
```

Build and check one platform with `make release-one VERSION=v0.1.0 OS=windows ARCH=amd64` and `make check-archive VERSION=v0.1.0 OS=windows ARCH=amd64`.

Builds and tests run in Docker as your user, with caches under `_output/`. Tests enable CGO for the race detector; release builds disable CGO for cross compilation. Set `TFWRAPPER_LIVE_TEST=1` when running `make test` to include the optional live tests.

ZIP archives and their `.sha256` files are written to `_output/release/`. Each archive contains the executable, `README.md`, and `CHANGELOG.md`.

| Platform | Archive |
| --- | --- |
| Linux amd64 | `tfwrapper-v0.1.0-linux-amd64.zip` |
| Linux arm64 | `tfwrapper-v0.1.0-linux-arm64.zip` |
| macOS arm64 | `tfwrapper-v0.1.0-darwin-arm64.zip` |
| Windows amd64 | `tfwrapper-v0.1.0-windows-amd64.zip` |

## Publish

1. Choose an unused stable version. Move the entries under `Unreleased` in `CHANGELOG.md` into a dated heading, such as `## [0.1.0] - 2026-11-05`, and keep an empty `Unreleased` section. Use `Added`, `Changed`, `Fixed`, and `Removed` headings as needed.
2. Add the release link, such as `[0.1.0]: https://github.com/cloudboss/tfwrapper/releases/tag/v0.1.0`, and update the `Unreleased` link to compare the new tag with `HEAD`.
3. Run the local checks and builds above, then run `make check-release VERSION=v0.1.0` to check for a dated changelog entry. The release action reads the matching section from `CHANGELOG.md` and rejects empty notes.
4. Commit the release changes and wait for branch CI to pass. Tag that commit and push the branch and tag:

   ```sh
   git tag -a v0.1.0 -m 'Release v0.1.0'
   git push origin HEAD
   git push origin v0.1.0
   ```

5. Check the release workflow and download the ZIP for your platform and its `.sha256` file. Verify it with `sha256sum -c tfwrapper-v0.1.0-linux-amd64.zip.sha256`, or use `shasum -a 256 -c` on macOS, then extract the ZIP.

The release job uses the repository `GITHUB_TOKEN` with `contents: write`. No extra release secret is required. Prerelease and other nonstandard tags do not publish releases.

If publication fails, inspect the GitHub Release for partial uploads before retrying. For source or changelog corrections, use a new commit and a new release version. Keep published tags unchanged.
