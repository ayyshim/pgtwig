package cli

import (
	"errors"
	"flag"
	"fmt"

	"pgtwig/internal/git"
	"pgtwig/internal/mysql"
	"pgtwig/internal/pg"
)

func (a *App) cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var dbName, defaultBranch, container, user, driver string
	fs.StringVar(&dbName, "db", "", "base database name")
	fs.StringVar(&defaultBranch, "default-branch", "", "default Git branch (inferred from HEAD if omitted)")
	fs.StringVar(&container, "container", "", "Docker container name")
	fs.StringVar(&driver, "driver", "psql", "database driver: psql or mysql")
	fs.StringVar(&user, "user", "", "database user (defaults to postgres for psql, root for mysql)")

	fs.Usage = func() {
		fmt.Fprintln(a.Out, "Usage: pgtwig init --db <name> [--driver psql|mysql] [--default-branch <branch>] [--container <name>] [--user <user>]")
		fs.PrintDefaults()
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") { fs.Usage(); return nil }
	if !a.Git.InsideWorkTree() { return errors.New("current directory is not inside a Git repository") }
	if help, err := a.parseFlags(fs, args); help || err != nil { return err }

	switch driver {
	case "psql", "postgres", "postgresql":
		driver = "psql"
		if err := pg.ValidateDBName(dbName); err != nil { return err }
		if user == "" { user = "postgres" }
	case "mysql":
		if err := mysql.ValidateDBName(dbName); err != nil { return err }
		if user == "" { user = "root" }
	default:
		return fmt.Errorf("invalid driver %q: expected psql or mysql", driver)
	}

	gitDir, err := a.Git.GitDir()
	if err != nil { return err }
	branch, err := a.Git.CurrentBranch("")
	if err != nil { return err }
	if defaultBranch == "" { defaultBranch = branch }

	cfg := git.Config{DBName: dbName, DefaultBranch: defaultBranch, CurrentBranch: branch, Driver: driver, User: user, Container: container}
	if err := a.Git.SaveConfig(cfg); err != nil { return err }
	hookPath, _, err := git.InstallHook(gitDir)
	if err != nil { return err }
	fmt.Fprintf(a.Out, "Initialized pgtwig (driver=%s, db=%s, default=%s, branch=%s, hook=%s)\n", cfg.Driver, cfg.DBName, cfg.DefaultBranch, cfg.CurrentBranch, hookPath)
	return nil
}
