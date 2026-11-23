package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"pgtwig/internal/git"
	"pgtwig/internal/pg"
)

type Git interface {
	InsideWorkTree() bool
	GitDir() (string, error)
	LoadConfig() (git.Config, error)
	SaveConfig(cfg git.Config) error
	SetConfig(key, val string) error
	CurrentBranch(fallbackSHA string) (string, error)
	Branches() ([]string, error)
}

type App struct {
	Git    Git
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	PG     func(cfg git.Config) pg.Client
	Docker func(container string) (bool, error)
}

func NewApp() *App {
	return &App{
		Git:    git.Client{},
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		PG: func(cfg git.Config) pg.Client {
			return pg.Client{User: cfg.User, Container: cfg.Container}
		},
		Docker: pg.ContainerRunning,
	}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp(a.Out)
		return 0
	}

	var err error
	switch args[0] {
	case "-h", "--help", "help":
		a.printHelp(a.Out)
		return 0
	case "init":
		err = a.cmdInit(args[1:])
	default:
		fmt.Fprintf(a.ErrOut, "Error: unknown command %q\n\n", args[0])
		a.printHelp(a.ErrOut)
		return 2
	}

	if err != nil {
		fmt.Fprintf(a.ErrOut, "Error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) parseFlags(fs *flag.FlagSet, args []string) (help bool, err error) {
	fs.SetOutput(a.Out)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, err
	}

	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return false, nil
}

func (a *App) printHelp(w io.Writer) {
	fmt.Fprint(w, `pgtwig - local PostgreSQL branching tied to Git branches

Usage:
  pgtwig <command> [flags]

Commands:
  init    Configure pgtwig in this repo and install the post-checkout hook
  list    Show the active database and branch snapshot status
  reset   Recreate the active database from the default branch snapshot
  clean   Drop orphaned branch databases

Run 'pgtwig <command> --help' for command options.
Set PGTWIG_PGPASSWORD if your container or local Postgres needs a password.
`)
}
