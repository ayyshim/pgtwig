package cli

import (
	"fmt"

	"pgtwig/internal/git"
	"pgtwig/internal/pg"
)

func SelectTemplate(
	dbName, prevBranch, defaultBranch, prevSHA, newSHA string,
	isAncestor func(prevSHA, newSHA string) bool,
	exists func(db string) (bool, error),
) (string, error) {
	prevSnap := pg.BranchDBName(dbName, prevBranch)
	defaultSnap := pg.BranchDBName(dbName, defaultBranch)

	first, second := defaultSnap, prevSnap
	if prevSHA != "" && newSHA != "" && isAncestor(prevSHA, newSHA) {
		first, second = prevSnap, defaultSnap
	}

	if ok, err := exists(first); err != nil {
		return "", err
	} else if ok {
		return first, nil
	}

	if second != first {
		if ok, err := exists(second); err != nil {
			return "", err
		} else if ok {
			return second, nil
		}
	}

	return "", fmt.Errorf("no template snapshot found (%s or %s)", first, second)
}

func (a *App) cmdHook(args []string) error {
	var prevSHA, newSHA string
	if len(args) >= 1 {
		prevSHA = args[0]
	}
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

	// openPG selects the configured database driver (PostgreSQL by default,
	// MySQL when configured) and checks any configured Docker container.
	db, err := a.openPG(cfg)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: database unavailable (%v), skipping swap\n", err)
		return nil
	}

	baseExists, err := db.Exists(cfg.DBName)
	if err != nil {
		fmt.Fprintf(a.ErrOut, "pgtwig: database unavailable (%v), skipping swap\n", err)
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
		tmpl, err := SelectTemplate(cfg.DBName, prevBranch, cfg.DefaultBranch, prevSHA, newSHA, a.Git.IsAncestor, db.Exists)
		if err != nil {
			_ = db.Rename(prevSnap, cfg.DBName)
			fmt.Fprintf(a.ErrOut, "pgtwig: %v\n", err)
			return nil
		}

		fmt.Fprintf(a.ErrOut, "pgtwig: cloning %q from %q\n", cfg.DBName, tmpl)
		if err := db.Clone(cfg.DBName, tmpl); err != nil {
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
