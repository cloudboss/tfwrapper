package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/hashicorp/go-version"
	"github.com/spf13/cobra"

	"github.com/cloudboss/tfwrapper/internal/root"
	"github.com/cloudboss/tfwrapper/internal/terraform"
)

func main() {
	installer := terraform.New(os.Stderr)
	if err := newCommand(installer.Ensure, execute).Execute(); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			os.Exit(exit.ExitCode())
		}
		_, _ = fmt.Fprintf(os.Stderr, "tfwrapper: %v\n", err)
		os.Exit(1)
	}
}

func newCommand(
	ensure func(context.Context, version.Constraints) (string, error),
	execute func(string, []string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   "tfwrapper [-chdir=path] <terraform command> [arguments]",
		Short: "Validate a root module and run its required Terraform version",
		Long: "Validate terraform.required_version and run the lowest matching Terraform release.\n" +
			"Terraform arguments are passed through unchanged.",
		DisableFlagParsing: true,
		SilenceErrors:      true,
		SilenceUsage:       true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(command *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			dir, err := root.Directory(cwd, args)
			if err != nil {
				return err
			}
			module, err := root.Load(dir)
			if err != nil {
				return fmt.Errorf("must run from a root module or define -chdir: %w", err)
			}
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			path, err := ensure(ctx, module.Constraints)
			if err == nil {
				err = ctx.Err()
			}
			stop()
			if err != nil {
				return err
			}
			return execute(path, args)
		},
	}
}
