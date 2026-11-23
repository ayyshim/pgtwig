package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const hookBlock = `# pgtwig post-checkout hook
if [ "$3" = "1" ]; then
  pgtwig _hook "$1" "$2"
fi
`

func MergeHook(existing string) (string, bool) {
	if strings.Contains(existing, "pgtwig _hook") {
		return existing, false
	}

	if strings.TrimSpace(existing) == "" {
		return "#!/bin/sh\n\n" + hookBlock, true
	}

	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + hookBlock, true
}

func InstallHook(gitDir string) (string, bool, error) {
	path := filepath.Join(gitDir, "hooks", "post-checkout")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, false, err
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return path, false, err
	}

	updated, changed := MergeHook(string(data))
	if changed {
		if err := os.WriteFile(path, []byte(updated), 0o755); err != nil {
			return path, false, err
		}
	}

	if err := os.Chmod(path, 0o755); err != nil {
		return path, changed, err
	}

	return path, changed, nil
}
