package pgtwig_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"pgtwig/internal/cli"
	"pgtwig/internal/pg"
)

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

func TestHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &cli.App{In: strings.NewReader(""), Out: &out, ErrOut: &errOut}

	if code := app.Run([]string{"--help"}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	for _, cmd := range []string{"pgtwig", "init", "list", "reset", "clean"} {
		if !strings.Contains(out.String(), cmd) {
			t.Fatalf("help missing %q", cmd)
		}
	}
}
