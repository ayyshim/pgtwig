package cli

import (
	"fmt"
	"io"
	"os"
)

type App struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

func NewApp() *App {
	return &App{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp(a.Out)
		return 0
	}

	switch args[0] {
	case "-h", "--help", "help":
		a.printHelp(a.Out)
		return 0
	default:
		fmt.Fprintf(a.ErrOut, "Error: unknown command %q\n\n", args[0])
		a.printHelp(a.ErrOut)
		return 2
	}
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
