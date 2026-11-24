package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"pgtwig/internal/git"
	"pgtwig/internal/mysql"
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
	IsAncestor(prevSHA, newSHA string) bool
}

type Database interface {
	Exists(string) (bool, error)
	List() (map[string]bool, error)
	ListPrefix(string) ([]string, error)
	Terminate(string) error
	Drop(string) error
	Rename(string, string) error
	Clone(string, string) error
}

type App struct {
	Git    Git
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
	PG     func(cfg git.Config) pg.Client
	MySQL  func(cfg git.Config) mysql.Client
	Docker func(container string) (bool, error)
}

func NewApp() *App {
	return &App{
		Git:    git.Client{},
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		PG: func(cfg git.Config) pg.Client { return pg.Client{User: cfg.User, Container: cfg.Container} },
		MySQL: func(cfg git.Config) mysql.Client { return mysql.Client{User: cfg.User, Container: cfg.Container} },
		Docker: pg.ContainerRunning,
	}
}

func (a *App) Run(args []string) int {
	if len(args) == 0 { a.printHelp(a.Out); return 0 }
	var err error
	switch args[0] {
	case "-h", "--help", "help": a.printHelp(a.Out); return 0
	case "init": err = a.cmdInit(args[1:])
	case "list": err = a.cmdList(args[1:])
	case "reset": err = a.cmdReset(args[1:])
	case "clean": err = a.cmdClean(args[1:])
	case "_hook": err = a.cmdHook(args[1:])
	default:
		fmt.Fprintf(a.ErrOut, "Error: unknown command %q\n\n", args[0])
		a.printHelp(a.ErrOut)
		return 2
	}
	if err != nil { fmt.Fprintf(a.ErrOut, "Error: %v\n", err); return 1 }
	return 0
}

func (a *App) parseFlags(fs *flag.FlagSet, args []string) (help bool, err error) {
	fs.SetOutput(a.Out)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) { return true, nil }
		return false, err
	}
	if fs.NArg() > 0 { return false, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")) }
	return false, nil
}

func (a *App) openPG(cfg git.Config) (Database, error) {
	if cfg.Container != "" {
		running, err := a.Docker(cfg.Container)
		if err != nil { return nil, fmt.Errorf("check container %s: %w", cfg.Container, err) }
		if !running { return nil, fmt.Errorf("container %s is not running", cfg.Container) }
	}
	if cfg.Driver == "mysql" {
		if a.MySQL != nil { return a.MySQL(cfg), nil }
		return mysql.Client{User: cfg.User, Container: cfg.Container}, nil
	}
	if a.PG != nil { return a.PG(cfg), nil }
	return pg.Client{User: cfg.User, Container: cfg.Container}, nil
}

func (a *App) confirm(prompt string) (bool, error) {
	fmt.Fprintf(a.Out, "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(a.In).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) { return false, err }
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes", nil
}

func (a *App) printHelp(w io.Writer) {
	fmt.Fprint(w, `pgtwig - local PostgreSQL and MySQL branching tied to Git branches

Usage:
  pgtwig <command> [flags]

Commands:
  init    Configure pgtwig in this repo and install the post-checkout hook
  list    Show the active database and branch snapshot status
  reset   Recreate the active database from the default branch snapshot
  clean   Drop orphaned branch databases

Run 'pgtwig <command> --help' for command options.
Set PGTWIG_PGPASSWORD for PostgreSQL or PGTWIG_MYSQL_PASSWORD for MySQL.
`)
}
