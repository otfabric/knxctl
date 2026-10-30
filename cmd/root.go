// SPDX-License-Identifier: MIT

// Package cmd is the command tree of knxctl.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/otfabric/go-knx"
	"github.com/otfabric/knxctl/internal/errs"
	"github.com/otfabric/knxctl/internal/output"
	"github.com/spf13/cobra"
)

var buildMeta struct{ version, tag, commit, buildDate string }

// SetBuildMeta records link-time metadata (main's -X flags).
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version, buildMeta.tag, buildMeta.commit, buildMeta.buildDate = version, tag, commit, buildDate
}

// app is the state of one invocation.
type app struct {
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
	format  output.Format
	started bool

	timeout time.Duration
	iface   string
	local   string
	verbose int
	quiet   bool
	logger  knx.Logger
}

// formatEnv returns the default of --format from the environment:
// KNXCTL_FORMAT, the canonical name, or else KNXCTL_OUTPUT_FORMAT.
func formatEnv() string {
	if v := os.Getenv("KNXCTL_FORMAT"); v != "" {
		return v
	}
	return os.Getenv("KNXCTL_OUTPUT_FORMAT")
}

// formatDefault is the default of --format.
func formatDefault() string {
	if v := formatEnv(); v != "" {
		return v
	}
	return "text"
}

// formatOf reads --format before anything else, so that usage errors can
// follow it too.
func formatOf(args []string) output.Format {
	v := formatEnv()
	for i, a := range args {
		switch {
		case a == "--":
			// Arguments after -- are not flags.
			return parseFormat(v)
		case a == "--format" && i+1 < len(args):
			v = args[i+1]
		case strings.HasPrefix(a, "--format="):
			v = strings.TrimPrefix(a, "--format=")
		}
	}
	return parseFormat(v)
}

func parseFormat(v string) output.Format {
	f, err := output.Parse(v)
	if err != nil {
		return output.Text
	}
	return f
}

func newRoot(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "knxctl",
		Short: "Discover, inspect, monitor and talk to KNX installations over KNXnet/IP",
		Long: `knxctl is a command-line tool for KNX over KNXnet/IP, built on
github.com/otfabric/go-knx.

Passive by default: discover, describe, monitor, group listen, routing monitor,
diagnose and the dpt and decode commands never send telegrams onto the KNX
network (discover sends one search request; describe and diagnose talk only to
the KNXnet/IP server). Commands that send telegrams are explicit: group write,
group respond, group answer and group read (a read is a telegram the devices
answer).

A KNXnet/IP server is reached by tunnelling (--gateway host[:port], env
KNXCTL_GATEWAY) or the KNX IP network by routing (--routing, multicast
224.0.23.12:3671 by default). --nat makes tunnelling work through NAT (Docker
port mappings) when the server supports route back endpoints.

knxctl serve runs a simulated KNXnet/IP server with virtual group objects.

Results go to stdout as text (default), a table or JSON (--format); diagnostics
go to stderr. Exit codes: docs/exitcodes.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.Parse(cmd.Flag("format").Value.String())
			if err != nil {
				return errs.Invalid("%v", err)
			}
			a.format = f
			a.started = true
			if a.timeout <= 0 {
				return errs.Invalid("--timeout %v: must be positive", a.timeout)
			}
			level := slog.LevelWarn
			switch {
			case a.quiet:
				level = slog.LevelError + 1
			case a.verbose >= 2:
				level = slog.LevelDebug
			case a.verbose == 1:
				level = slog.LevelInfo
			}
			a.logger = knx.NewSlogLogger(slog.NewTextHandler(a.errOut, &slog.HandlerOptions{Level: level}))
			return nil
		},
	}
	pf := root.PersistentFlags()
	pf.String("format", formatDefault(), "Output format: text, json or table (env KNXCTL_FORMAT)")
	_ = root.RegisterFlagCompletionFunc("format", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"text\thuman-readable", "json\tversioned JSON", "table\taligned columns"}, cobra.ShellCompDirectiveNoFileComp
	})
	pf.DurationVar(&a.timeout, "timeout", 5*time.Second, "Timeout of each request (search window, connect, read)")
	pf.StringVarP(&a.iface, "interface", "i", "", "Network interface for multicast and the announced address")
	pf.StringVar(&a.local, "local-addr", "", "Local address to bind (IP or IP:port)")
	pf.CountVarP(&a.verbose, "verbose", "v", "Log protocol events to stderr (-v info, -vv debug)")
	pf.BoolVarP(&a.quiet, "quiet", "q", false, "Log nothing to stderr")
	root.AddCommand(newDiscoverCmd(a), newDescribeCmd(a), newTunnelCmd(a), newGroupCmd(a), newMonitorCmd(a), newRoutingCmd(a),
		newDPTCmd(a), newDecodeCmd(a), newDiagnoseCmd(a), newBusmonitorCmd(a), newPropertyCmd(a), newKeyringCmd(a), newServeCmd(a), newVersionCmd(a), newCompletionCmd())
	return root
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// emit writes a result in the selected format.
func (a *app) emit(command string, r output.Result) error {
	if err := output.Write(a.out, a.format, command, r); err != nil {
		return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
	}
	return nil
}

// stream returns a record stream on stdout.
func (a *app) stream() *output.Stream { return output.NewStream(a.out, a.format) }

// warn writes a diagnostic line to stderr.
func (a *app) warn(format string, args ...any) {
	if !a.quiet {
		_, _ = fmt.Fprintf(a.errOut, "knxctl: "+format+"\n", args...)
	}
}

// Run executes knxctl with args and returns the exit code.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	a := &app{in: in, out: out, errOut: errOut, format: formatOf(args), logger: knx.NopLogger()}
	root := newRoot(a)
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return errs.ExitOK
	}
	if errors.Is(err, errs.ErrFindings) {
		return errs.ExitFindings
	}
	var e *errs.Error
	if !a.started || isUsage(err) {
		e = &errs.Error{Kind: errs.KindUsage, Code: errs.CodeInvalidInput, Message: err.Error(), Cause: err}
		var typed *errs.Error
		if errors.As(err, &typed) {
			e = typed
		}
	} else {
		e = errs.Classify(err)
	}
	if a.format == output.JSON {
		_ = output.WriteError(out, e)
	} else {
		_, _ = fmt.Fprintf(errOut, "error: %s\n", e.Message)
		if strings.Contains(e.Message, "unknown shorthand flag") && negativeArg(args) {
			_, _ = fmt.Fprintln(errOut, "A negative value is read as a flag: put -- before the arguments, for example knxctl group write --dpt 9.001 -g host -- 1/2/4 -30")
		}
		if e.Kind == errs.KindUsage {
			_, _ = fmt.Fprintln(errOut, "Run 'knxctl --help' for usage.")
		}
	}
	return errs.ExitCode(e.Kind)
}

// negativeArg reports an argument that looks like a negative number.
func negativeArg(args []string) bool {
	for _, a := range args {
		if len(a) > 1 && a[0] == '-' && (a[1] >= '0' && a[1] <= '9' || a[1] == '.') {
			return true
		}
	}
	return false
}

// isUsage recognises cobra's own errors about the command line.
func isUsage(err error) bool {
	var typed *errs.Error
	if errors.As(err, &typed) {
		return false
	}
	msg := err.Error()
	for _, p := range []string{"required flag", "accepts ", "requires at least", "unknown command", "unknown flag",
		"unknown shorthand", "invalid argument", "flag needs an argument", "if any flags in the group"} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// Execute runs knxctl on the process's arguments, ending on SIGINT or
// SIGTERM.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

type versionResult struct {
	Version   string `json:"version"`
	Tag       string `json:"tag,omitempty"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
	GoKNX     string `json:"goKnx"`
	Go        string `json:"go"`
	Platform  string `json:"platform"`
}

func (v versionResult) Text() string {
	s := "knxctl " + v.Version
	if v.Commit != "" {
		s += " (" + v.Commit + ")"
	}
	return s + "\ngo-knx " + v.GoKNX + "\n" + v.Go + " " + v.Platform
}

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of knxctl and go-knx",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			v := versionResult{Version: buildMeta.version, Tag: buildMeta.tag, Commit: buildMeta.commit, BuildDate: buildMeta.buildDate,
				GoKNX: "(unknown)", Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
			if v.Version == "" {
				v.Version = "dev"
			}
			if info, ok := debug.ReadBuildInfo(); ok {
				// "go install github.com/otfabric/knxctl@vX.Y.Z" sets no
				// link-time version; the module version says it.
				if v.Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
					v.Version = info.Main.Version
				}
				for _, d := range info.Deps {
					if d.Path == "github.com/otfabric/go-knx" {
						v.GoKNX = d.Version
					}
				}
			}
			return a.emit("version", v)
		},
	}
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion bash|zsh|fish|powershell",
		Short:     "Generate a shell completion script",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return errs.Invalid("unknown shell %q", args[0])
		},
	}
}
