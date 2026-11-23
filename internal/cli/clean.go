package cli

import (
	"flag"
	"fmt"
	"strings"

	"pgtwig/internal/pg"
)

func FindOrphans(dbName string, dbs, branches []string) []string {
	prefix := dbName + "-"
	keep := make(map[string]bool, len(branches))
	for _, b := range branches {
		keep[pg.BranchHash(dbName, b)] = true
	}

	var orphans []string
	for _, db := range dbs {
		if suffix, ok := strings.CutPrefix(db, prefix); ok && !keep[suffix] {
			orphans = append(orphans, db)
		}
	}
	return orphans
}

func (a *App) cmdClean(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)

	var force bool
	fs.BoolVar(&force, "force", false, "skip confirmation")
	fs.BoolVar(&force, "f", false, "skip confirmation")

	fs.Usage = func() {
		fmt.Fprintln(a.Out, "Usage: pgtwig clean [--force]")
		fs.PrintDefaults()
	}

	if help, err := a.parseFlags(fs, args); help || err != nil {
		return err
	}

	cfg, err := a.Git.LoadConfig()
	if err != nil {
		return err
	}

	db, err := a.openPG(cfg)
	if err != nil {
		return err
	}

	candidates, err := db.ListPrefix(cfg.DBName + "-")
	if err != nil {
		return err
	}

	branches, err := a.Git.Branches()
	if err != nil {
		return err
	}

	orphans := FindOrphans(cfg.DBName, candidates, branches)
	if len(orphans) == 0 {
		fmt.Fprintln(a.Out, "No orphaned databases found.")
		return nil
	}

	for _, name := range orphans {
		fmt.Fprintf(a.Out, "  %s\n", name)
	}

	if !force {
		confirmed, err := a.confirm(fmt.Sprintf("Drop %d orphaned database(s)?", len(orphans)))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(a.Out, "Aborted.")
			return nil
		}
	}

	for _, name := range orphans {
		if err := db.Drop(name); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Dropped %s\n", name)
	}

	return nil
}
