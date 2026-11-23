package pgtwig_test

import (
	"bytes"
	"strings"
	"testing"

	"pgtwig/internal/cli"
)

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
