# tfwrapper

Run Terraform with the version declared by the root module. tfwrapper chooses the lowest published version allowed by `terraform.required_version`, caches the executable, and runs it.

## Build

Use the Go version declared in [go.mod](go.mod).

```sh
go build -o bin/tfwrapper .
```

Put `tfwrapper` on your PATH, then run it from a Terraform root module:

```sh
tfwrapper init
tfwrapper plan -out=plan.out
tfwrapper apply plan.out
```

You can also use Terraform's `-chdir` option to run from another directory:

```sh
tfwrapper -chdir=envs/dev plan
```

All arguments are passed through to Terraform. Without a valid root module, `tfwrapper` has no terraform version to run, so it fails with an error message.

```sh
tfwrapper: must run from a root module or define -chdir: root module /path/to/cwd must contain a terraform block
```

## Root module requirements

The root module must contain a `terraform` block with a nonempty version constraint:

```hcl
terraform {
  required_version = ">= 1.15.0, < 2.0.0"
}
```

All top level `.tf` and `.tf.json` files are parsed together. If `required_version` is a range, the lowest published release is chosen that satisfies the constraint. Strict bounds and exclusions are respected. Prereleases require an exact constraint.

Any version requirements defined in child modules are checked by Terraform when it loads those modules, not by tfwrapper.

## Caching

Terraform executables are cached and run from the operating system's user cache directory in `tfwrapper/<os>_<architecture>`.

## Tests

```sh
go test -race ./...
go vet ./...
```

Optional live tests download an official Terraform release and run tests against it:

```sh
TFWRAPPER_LIVE_TEST=1 go test ./... -run TestLive -count=1
```
