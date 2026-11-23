package cli

import (
	"flag"
	"fmt"

	"pgtwig/internal/pg"
)

func (a *App) cmdReset(args []string) error {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)

	var force bool
	fs.BoolVar(&force, "force", false, "skip confirmation")
	fs.BoolVar(&force, "f", false, "skip confirmation")

	fs.Usage = func() {
		fmt.Fprintln(a.Out, "Usage: pgtwig reset [--force]")
		fs.PrintDefaults()
	}

	if help, err := a.parseFlags(fs, args); help || err != nil {
		return err
	}

	cfg, err := a.Git.LoadConfig()
	if err != nil {
		return err
	}

	branch, err := a.Git.CurrentBranch("")
	if err != nil {
		branch = cfg.CurrentBranch
	}

	if branch == cfg.DefaultBranch {
		fmt.Fprintf(a.Out, "Already on default branch %q; nothing to reset.\n", cfg.DefaultBranch)
		return nil
	}

	db, err := a.openPG(cfg)
	if err != nil {
		return err
	}

	defaultSnap := pg.BranchDBName(cfg.DBName, cfg.DefaultBranch)
	ok, err := db.Exists(defaultSnap)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("default branch snapshot %q not found", defaultSnap)
	}

	if !force {
		confirmed, err := a.confirm(fmt.Sprintf("Drop %q and recreate from %q?", cfg.DBName, defaultSnap))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(a.Out, "Aborted.")
			return nil
		}
	}

	if err := db.Drop(cfg.DBName); err != nil {
		return err
	}
	if err := db.Clone(cfg.DBName, defaultSnap); err != nil {
		return err
	}

	fmt.Fprintf(a.Out, "Reset %q from %q.\n", cfg.DBName, defaultSnap)
	return nil
}
