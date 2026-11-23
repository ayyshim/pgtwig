package pgtwig_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"pgtwig/internal/cli"
	"pgtwig/internal/git"
	"pgtwig/internal/pg"
)

type stubGit struct {
	inRepo     bool
	dir        string
	branch     string
	branches   []string
	cfg        git.Config
	saved      bool
	isAncestor bool
}

func (s *stubGit) InsideWorkTree() bool { return s.inRepo }

func (s *stubGit) GitDir() (string, error) {
	if !s.inRepo {
		return "", errors.New("current directory is not inside a Git repository")
	}
	return s.dir, nil
}

func (s *stubGit) LoadConfig() (git.Config, error) { return s.cfg, nil }

func (s *stubGit) SaveConfig(cfg git.Config) error {
	s.saved = true
	s.cfg = cfg
	return nil
}

func (s *stubGit) SetConfig(key, val string) error {
	if key == git.KeyCurrentBranch {
		s.cfg.CurrentBranch = val
	}
	return nil
}

func (s *stubGit) CurrentBranch(fallback string) (string, error) {
	if s.branch != "" {
		return s.branch, nil
	}
	return fallback, nil
}

func (s *stubGit) Branches() ([]string, error) { return s.branches, nil }

func (s *stubGit) IsAncestor(prevSHA, newSHA string) bool { return s.isAncestor }

func newTestApp(g *stubGit, execSQL func(string) (string, error), dockerRunning bool) (*cli.App, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	app := &cli.App{
		Git:    g,
		In:     strings.NewReader(""),
		Out:    &out,
		ErrOut: &errOut,
		PG: func(cfg git.Config) pg.Client {
			return pg.Client{User: cfg.User, Container: cfg.Container, Exec: execSQL}
		},
		Docker: func(string) (bool, error) { return dockerRunning, nil },
	}
	return app, &out, &errOut
}

func TestBranchHash(t *testing.T) {
	sum := sha1.Sum([]byte("feature/auth"))
	want := hex.EncodeToString(sum[:])

	got := pg.BranchHash("myapp", "feature/auth")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if pg.BranchHash("myapp", "main") == got {
		t.Fatal("different branches produced the same hash")
	}
}

func TestDBNameLimit(t *testing.T) {
	cases := []struct {
		db      string
		hashLen int
		fullLen int
	}{
		{"app", 40, 44},
		{strings.Repeat("a", 22), 40, 63},
		{strings.Repeat("b", 23), 39, 63},
		{strings.Repeat("c", 40), 22, 63},
		{strings.Repeat("d", 54), 8, 63},
		{strings.Repeat("e", 60), 8, 69},
	}

	for _, tc := range cases {
		if got := pg.HashLen(tc.db); got != tc.hashLen {
			t.Errorf("HashLen(%d bytes) = %d, want %d", len(tc.db), got, tc.hashLen)
		}
		if got := len(pg.BranchDBName(tc.db, "feature/x")); got != tc.fullLen {
			t.Errorf("len(BranchDBName(%d bytes)) = %d, want %d", len(tc.db), got, tc.fullLen)
		}
	}

	if err := pg.ValidateDBName(strings.Repeat("x", 54)); err != nil {
		t.Fatalf("54 bytes should be valid: %v", err)
	}
	if err := pg.ValidateDBName(strings.Repeat("x", 55)); err == nil {
		t.Fatal("55 bytes should fail")
	}
	if err := pg.ValidateDBName(""); err == nil {
		t.Fatal("empty name should fail")
	}
}

func TestSQLQuoting(t *testing.T) {
	if got := pg.QuoteIdent(`my"db`); got != `"my""db"` {
		t.Errorf("QuoteIdent = %q", got)
	}
	if got := pg.QuoteIdent(`""a""`); got != `"""""a"""""` {
		t.Errorf("QuoteIdent = %q", got)
	}
	if got := pg.QuoteLiteral("o'reilly"); got != `'o''reilly'` {
		t.Errorf("QuoteLiteral = %q", got)
	}
}

func TestHookInstall(t *testing.T) {
	dir := t.TempDir()

	path, changed, err := git.InstallHook(dir)
	if err != nil || !changed {
		t.Fatalf("first install: changed=%v err=%v", changed, err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "#!/bin/sh\n") || !strings.Contains(string(body), `pgtwig _hook "$1" "$2"`) {
		t.Fatalf("unexpected hook:\n%s", body)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("hook not executable: %o", info.Mode().Perm())
		}
	}

	_, changed, err = git.InstallHook(dir)
	if err != nil || changed {
		t.Fatalf("reinstall should be a no-op: changed=%v err=%v", changed, err)
	}

	// Appending preserves existing content without trailing newline.
	dir2 := t.TempDir()
	hook2 := filepath.Join(dir2, "hooks", "post-checkout")
	_ = os.MkdirAll(filepath.Dir(hook2), 0o755)
	_ = os.WriteFile(hook2, []byte("#!/bin/sh\necho hi"), 0o644)

	if _, changed, err := git.InstallHook(dir2); err != nil || !changed {
		t.Fatalf("append install: changed=%v err=%v", changed, err)
	}
	merged, _ := os.ReadFile(hook2)
	if !strings.HasPrefix(string(merged), "#!/bin/sh\necho hi\n") || strings.Count(string(merged), "pgtwig _hook") != 1 {
		t.Fatalf("existing hook not preserved:\n%s", merged)
	}
}

func TestPsqlCommand(t *testing.T) {
	bin, args, env := pg.BuildCommand("my-pg", "postgres", "secret")
	if bin != "docker" || len(env) != 0 {
		t.Fatalf("bin=%q env=%v", bin, env)
	}
	for _, a := range args {
		if a == "-t" || a == "-it" || a == "--tty" {
			t.Fatalf("unexpected tty flag in %v", args)
		}
	}
	if args[0] != "exec" || args[1] != "-i" || !strings.Contains(strings.Join(args, " "), "-e PGPASSWORD=secret") {
		t.Fatalf("unexpected docker args: %v", args)
	}

	bin, args, env = pg.BuildCommand("", "alice", "pw")
	if bin != "psql" || len(env) != 1 || env[0] != "PGPASSWORD=pw" {
		t.Fatalf("bin=%q args=%v env=%v", bin, args, env)
	}
}

func TestCloneFallback(t *testing.T) {
	var queries []string
	c := pg.Client{
		Exec: func(sql string) (string, error) {
			queries = append(queries, strings.TrimSpace(sql))
			if strings.Contains(sql, "STRATEGY = FILE_COPY") {
				return "", errors.New(`ERROR: syntax error at or near "STRATEGY"`)
			}
			return "", nil
		},
	}

	if err := c.Clone("myapp", "myapp-abc12345"); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || queries[2] != `CREATE DATABASE "myapp" TEMPLATE "myapp-abc12345";` {
		t.Fatalf("unexpected queries: %v", queries)
	}
}

func TestInitOutsideGit(t *testing.T) {
	dir := t.TempDir()
	g := &stubGit{inRepo: false, dir: filepath.Join(dir, ".git")}
	app, _, errOut := newTestApp(g, nil, true)

	code := app.Run([]string{"init", "--db", "myapp"})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "Error: current directory is not inside a Git repository") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
	if g.saved {
		t.Fatal("config should not be saved outside a git repo")
	}
}

func TestInitDefaultBranch(t *testing.T) {
	// Infers default branch from HEAD when omitted.
	g := &stubGit{inRepo: true, dir: t.TempDir(), branch: "develop"}
	app, _, errOut := newTestApp(g, nil, true)

	if code := app.Run([]string{"init", "--db", "shop"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if g.cfg.DefaultBranch != "develop" || g.cfg.CurrentBranch != "develop" {
		t.Fatalf("unexpected branches: %+v", g.cfg)
	}

	// Respects explicit --default-branch.
	g2 := &stubGit{inRepo: true, dir: t.TempDir(), branch: "feature/cart"}
	app2, _, errOut2 := newTestApp(g2, nil, true)

	if code := app2.Run([]string{"init", "--db", "shop", "--default-branch", "main", "--container", "pg16"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut2.String())
	}
	if g2.cfg.DefaultBranch != "main" || g2.cfg.CurrentBranch != "feature/cart" || g2.cfg.Container != "pg16" {
		t.Fatalf("unexpected config: %+v", g2.cfg)
	}
}

func TestSelectTemplate(t *testing.T) {
	prevSnap := pg.BranchDBName("myapp", "feature/a")
	defaultSnap := pg.BranchDBName("myapp", "main")

	cases := []struct {
		name       string
		isAncestor bool
		dbs        map[string]bool
		want       string
	}{
		{"ancestor exists", true, map[string]bool{prevSnap: true, defaultSnap: true}, prevSnap},
		{"ancestor missing", true, map[string]bool{defaultSnap: true}, defaultSnap},
		{"non-ancestor uses default", false, map[string]bool{prevSnap: true, defaultSnap: true}, defaultSnap},
		{"non-ancestor fallback to prev", false, map[string]bool{prevSnap: true}, prevSnap},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.SelectTemplate(
				"myapp", "feature/a", "main", "111", "222",
				func(string, string) bool { return tc.isAncestor },
				func(name string) (bool, error) { return tc.dbs[name], nil },
			)
			if err != nil || got != tc.want {
				t.Fatalf("got (%q, %v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestHookSwap(t *testing.T) {
	t.Run("stopped container skips swap", func(t *testing.T) {
		g := &stubGit{
			branch: "feature/a",
			cfg:    git.Config{DBName: "myapp", DefaultBranch: "main", CurrentBranch: "main", Container: "pg16"},
		}
		calledSQL := false
		app, _, _ := newTestApp(g, func(string) (string, error) { calledSQL = true; return "", nil }, false)

		if code := app.Run([]string{"_hook", "111", "222"}); code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if calledSQL || g.cfg.CurrentBranch != "main" {
			t.Fatalf("should not touch pg or branch when container is stopped")
		}
	})

	t.Run("missing base db updates branch", func(t *testing.T) {
		g := &stubGit{
			branch: "feature/a",
			cfg:    git.Config{DBName: "myapp", DefaultBranch: "main", CurrentBranch: "main"},
		}
		app, _, _ := newTestApp(g, func(string) (string, error) { return "", nil }, true)

		app.Run([]string{"_hook", "111", "222"})
		if g.cfg.CurrentBranch != "feature/a" {
			t.Fatalf("CurrentBranch = %q, want feature/a", g.cfg.CurrentBranch)
		}
	})

	t.Run("non-ancestor clones from default snapshot", func(t *testing.T) {
		prevSnap := pg.BranchDBName("myapp", "feature/a")
		defaultSnap := pg.BranchDBName("myapp", "main")
		dbs := map[string]bool{"myapp": true, defaultSnap: true}
		var cloneSQL string

		g := &stubGit{
			branch:     "feature/b",
			isAncestor: false,
			cfg:        git.Config{DBName: "myapp", DefaultBranch: "main", CurrentBranch: "feature/a"},
		}
		app, _, errOut := newTestApp(g, func(sql string) (string, error) {
			if strings.HasPrefix(sql, "SELECT 1") {
				for name := range dbs {
					if strings.Contains(sql, pg.QuoteLiteral(name)) {
						return "1", nil
					}
				}
				return "", nil
			}
			if strings.Contains(sql, "RENAME TO "+pg.QuoteIdent(prevSnap)) {
				delete(dbs, "myapp")
				dbs[prevSnap] = true
			}
			if strings.HasPrefix(sql, "CREATE DATABASE") {
				cloneSQL = sql
				dbs["myapp"] = true
			}
			return "", nil
		}, true)

		app.Run([]string{"_hook", "111", "222"})
		if g.cfg.CurrentBranch != "feature/b" {
			t.Fatalf("CurrentBranch = %q, want feature/b", g.cfg.CurrentBranch)
		}
		if !strings.Contains(cloneSQL, pg.QuoteIdent(defaultSnap)) || !strings.Contains(errOut.String(), defaultSnap) {
			t.Fatalf("expected clone from %q, sql=%q log=%q", defaultSnap, cloneSQL, errOut.String())
		}
	})

	t.Run("restores existing snapshot", func(t *testing.T) {
		prevSnap := pg.BranchDBName("myapp", "feature/a")
		targetSnap := pg.BranchDBName("myapp", "main")
		dbs := map[string]bool{"myapp": true, targetSnap: true}

		g := &stubGit{
			branch: "main",
			cfg:    git.Config{DBName: "myapp", DefaultBranch: "main", CurrentBranch: "feature/a"},
		}
		app, _, _ := newTestApp(g, func(sql string) (string, error) {
			if strings.HasPrefix(sql, "SELECT 1") {
				for name := range dbs {
					if strings.Contains(sql, pg.QuoteLiteral(name)) {
						return "1", nil
					}
				}
				return "", nil
			}
			if strings.Contains(sql, "RENAME TO "+pg.QuoteIdent(prevSnap)) {
				delete(dbs, "myapp")
				dbs[prevSnap] = true
			}
			if strings.Contains(sql, pg.QuoteIdent(targetSnap)+" RENAME TO") {
				delete(dbs, targetSnap)
				dbs["myapp"] = true
			}
			return "", nil
		}, true)

		app.Run([]string{"_hook", "111", "222"})
		if !dbs["myapp"] || !dbs[prevSnap] || dbs[targetSnap] || g.cfg.CurrentBranch != "main" {
			t.Fatalf("unexpected state: dbs=%v branch=%q", dbs, g.cfg.CurrentBranch)
		}
	})
}

func TestFindOrphans(t *testing.T) {
	mainSnap := pg.BranchDBName("app", "main")
	oldSnap := pg.BranchDBName("app", "old-branch")

	got := cli.FindOrphans("app", []string{mainSnap, oldSnap}, []string{"main"})
	if len(got) != 1 || got[0] != oldSnap {
		t.Fatalf("got %v, want [%s]", got, oldSnap)
	}
}

func TestHelp(t *testing.T) {
	app, out, _ := newTestApp(&stubGit{}, nil, true)
	if code := app.Run([]string{"--help"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	for _, cmd := range []string{"pgtwig", "init", "list", "reset", "clean"} {
		if !strings.Contains(out.String(), cmd) {
			t.Fatalf("help missing %q", cmd)
		}
	}
}
