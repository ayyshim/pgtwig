package cli

import (
	"flag"
	"fmt"
	"text/tabwriter"

	"pgtwig/internal/pg"
)

func (a *App) cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(a.Out, "Usage: pgtwig list")
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

	dbs, err := db.List()
	if err != nil {
		return err
	}

	branches, err := a.Git.Branches()
	if err != nil {
		return err
	}

	active, err := a.Git.CurrentBranch("")
	if err != nil {
		active = cfg.CurrentBranch
	}

	state := "present"
	if !dbs[cfg.DBName] {
		state = "missing"
	}
	fmt.Fprintf(a.Out, "Active DB: %s (%s) on %s\n\n", cfg.DBName, state, active)

	tw := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BRANCH\tDATABASE\tSTATUS")

	for _, b := range branches {
		mapped := pg.BranchDBName(cfg.DBName, b)

		status := "unbranched"
		if b == active {
			status = "active"
		} else if dbs[mapped] {
			status = "snapshot exists"
		}

		mark := "  "
		if b == active {
			mark = "* "
		}

		fmt.Fprintf(tw, "%s%s\t%s\t%s\n", mark, b, mapped, status)
	}

	return tw.Flush()
}
