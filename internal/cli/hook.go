package cli

import (
	"fmt"

	"pgtwig/internal/git"
	"pgtwig/internal/pg"
)

func (a *App) cmdHook(args []string) error {
	var newSHA string
	if len(args) >= 2 {
		newSHA = args[1]
	}

	cfg, err := a.Git.LoadConfig()
	if err != nil {
		return nil
	}

	newBranch, err := a.Git.CurrentBranch(newSHA)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: %v\n", err)
		return nil
	}

	prevBranch := cfg.CurrentBranch
	if prevBranch == newBranch {
		return nil
	}

	if cfg.Container != "" {
		running, err := a.Docker(cfg.Container)
		if err != nil || !running {
			fmt.Fprintf(a.ErrOut, "pgtwig: container %q is not running, skipping swap\n", cfg.Container)
			return nil
		}
	}

	db := a.PG(cfg)

	baseExists, err := db.Exists(cfg.DBName)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: postgres unreachable (%v), skipping swap\n", err)
		return nil
	}
	if !baseExists {
		fmt.Fprintf(a.ErrOut, "pgtwig: database %q missing, updating branch to %q\n", cfg.DBName, newBranch)
		_ = a.Git.SetConfig(git.KeyCurrentBranch, newBranch)
		return nil
	}

	if prevBranch == "" {
		_ = a.Git.SetConfig(git.KeyCurrentBranch, newBranch)
		return nil
	}

	prevSnap := pg.BranchDBName(cfg.DBName, prevBranch)
	newSnap := pg.BranchDBName(cfg.DBName, newBranch)

	if err := db.Drop(prevSnap); err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: failed to drop stale %q: %v\n", prevSnap, err)
		return nil
	}
	if err := db.Rename(cfg.DBName, prevSnap); err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: failed to park %q: %v\n", cfg.DBName, err)
		return nil
	}

	hasTarget, err := db.Exists(newSnap)
	if err != nil {
		_ = db.Rename(prevSnap, cfg.DBName)
		fmt.Fprintf(a.ErrOut, "pgtwig: failed to check %q: %v\n", newSnap, err)
		return nil
	}

	if hasTarget {
		if err := db.Rename(newSnap, cfg.DBName); err != nil {
			_ = db.Rename(prevSnap, cfg.DBName)
			fmt.Fprintf(a.ErrOut, "pgtwig: failed to restore %q: %v\n", newSnap, err)
			return nil
		}
		fmt.Fprintf(a.ErrOut, "pgtwig: restored %q for branch %q\n", cfg.DBName, newBranch)
	} else {
		fmt.Fprintf(a.ErrOut, "pgtwig: cloning %q from %q\n", cfg.DBName, prevSnap)
		if err := db.Clone(cfg.DBName, prevSnap); err != nil {
			_ = db.Rename(prevSnap, cfg.DBName)
			fmt.Fprintf(a.ErrOut, "pgtwig: clone failed: %v\n", err)
			return nil
		}
	}

	if err := a.Git.SetConfig(git.KeyCurrentBranch, newBranch); err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: failed to save current branch: %v\n", err)
	}
	return nil
}
