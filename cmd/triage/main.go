// Command triage explores Jev review questions against a patch read from stdin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	jev "github.com/withzombies/jev-go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	code := command(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	cancel()
	os.Exit(code)
}

// command is the composition root. It may close its process input on cancellation
// to interrupt a blocked read. The library and run never read process state.
// Diagnostic writes are best effort: their failures cannot be reported on stderr.
func command(ctx context.Context, args []string, in io.ReadCloser, out, stderr io.Writer, getenv func(string) string) int {
	stop := context.AfterFunc(ctx, func() {
		// Closing is best effort; the interrupted read supplies the operation error.
		_ = in.Close()
	})
	defer stop()
	var opts options
	flags := flag.NewFlagSet("triage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.model, "model", jev.DefaultModel, "Jev model name or alias")
	flags.Int64Var(&opts.contextBytes, "context-bytes", defaultContextBytes, "maximum stdin prefix bytes to evaluate (not a token count)")
	flags.BoolVar(&opts.jsonOutput, "json", false, "print a JSON report")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: gh pr diff 8556 | triage [--model MODEL] [--json] [--context-bytes N]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "triage: provide the diff on stdin, not as an argument")
		return 1
	}
	if opts.contextBytes <= 0 {
		_, _ = fmt.Fprintln(stderr, "triage: --context-bytes must be positive")
		return 1
	}
	client, err := jev.NewClient(jev.Config{APIKey: getenv("TYPESAFE_API_KEY")})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "triage: configure TYPESAFE_API_KEY: %v\n", err)
		return 1
	}
	if err := run(ctx, client, in, out, opts); err != nil {
		_, _ = fmt.Fprintf(stderr, "triage: %v\n", err)
		return 1
	}
	return 0
}
