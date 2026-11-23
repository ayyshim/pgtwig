package cli

import (
	"errors"
	"flag"
	"fmt"

	"pgtwig/internal/git"
	"pgtwig/internal/pg"
)

func (a *App) cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)

	var dbName, defaultBranch, container, user string
	fs.StringVar(&dbName, "db", "", "base PostgreSQL database name")
	fs.StringVar(&defaultBranch, "default-branch", "", "default Git branch (inferred from HEAD if omitted)")
	fs.StringVar(&container, "container", "", "Docker container name")
	fs.StringVar(&user, "user", "postgres", "PostgreSQL user")

	fs.Usage = func() {
		fmt.Fprintln(a.Out, "Usage: pgtwig init --db <name> [--default-branch <branch>] [--container <name>] [--user <user>]")
		fs.PrintDefaults()
	}

	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return nil
	}

	if !a.Git.InsideWorkTree() {
		return errors.New("current directory is not inside a Git repository")
	}

	if help, err := a.parseFlags(fs, args); help || err != nil {
		return err
	}

	if err := pg.ValidateDBName(dbName); err != nil {
		return err
	}

	gitDir, err := a.Git.GitDir()
	if err != nil {
		return err
	}

	branch, err := a.Git.CurrentBranch("")
	if err != nil {
		return err
	}

	if defaultBranch == "" {
		defaultBranch = branch
	}
	if user == "" {
		user = "postgres"
	}

	cfg := git.Config{
		DBName:        dbName,
		DefaultBranch: defaultBranch,
		CurrentBranch: branch,
		User:          user,
		Container:     container,
	}

	if err := a.Git.SaveConfig(cfg); err != nil {
		return err
	}

	hookPath, _, err := git.InstallHook(gitDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(a.Out, "Initialized pgtwig (db=%s, default=%s, branch=%s, hook=%s)\n",
		cfg.DBName, cfg.DefaultBranch, cfg.CurrentBranch, hookPath)
	return nil
}
