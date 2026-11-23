package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	KeyDBName        = "pgtwig.dbname"
	KeyDefaultBranch = "pgtwig.defaultbranch"
	KeyCurrentBranch = "pgtwig.currentbranch"
	KeyUser          = "pgtwig.user"
	KeyContainer     = "pgtwig.container"
)

type Config struct {
	DBName        string
	DefaultBranch string
	CurrentBranch string
	User          string
	Container     string
}

type Client struct {
	Dir string
}

func (c Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Dir

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", err
	}

	return strings.TrimSpace(out.String()), nil
}

func (c Client) InsideWorkTree() bool {
	out, err := c.run("rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

func (c Client) GitDir() (string, error) {
	dir, err := c.run("rev-parse", "--git-dir")
	if err != nil {
		return "", errors.New("current directory is not inside a Git repository")
	}
	if !filepath.IsAbs(dir) && c.Dir != "" {
		return filepath.Clean(filepath.Join(c.Dir, dir)), nil
	}
	return filepath.Clean(dir), nil
}

func (c Client) get(key string) (string, error) {
	out, err := c.run("config", "--local", "--get", key)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return out, nil
}

func (c Client) SetConfig(key, val string) error {
	_, err := c.run("config", "--local", key, val)
	return err
}

func (c Client) unset(key string) error {
	_, err := c.run("config", "--local", "--unset", key)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
			return nil
		}
		return err
	}
	return nil
}

func (c Client) LoadConfig() (Config, error) {
	if _, err := c.GitDir(); err != nil {
		return Config{}, err
	}

	var cfg Config
	var err error

	if cfg.DBName, err = c.get(KeyDBName); err != nil {
		return cfg, err
	}
	if cfg.DBName == "" {
		return cfg, errors.New("pgtwig is not initialized; run 'pgtwig init --db <name>' first")
	}

	if cfg.DefaultBranch, err = c.get(KeyDefaultBranch); err != nil {
		return cfg, err
	}
	if cfg.DefaultBranch == "" {
		cfg.DefaultBranch = "main"
	}

	if cfg.CurrentBranch, err = c.get(KeyCurrentBranch); err != nil {
		return cfg, err
	}
	if cfg.User, err = c.get(KeyUser); err != nil {
		return cfg, err
	}
	if cfg.Container, err = c.get(KeyContainer); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (c Client) SaveConfig(cfg Config) error {
	entries := [][2]string{
		{KeyDBName, cfg.DBName},
		{KeyDefaultBranch, cfg.DefaultBranch},
		{KeyCurrentBranch, cfg.CurrentBranch},
		{KeyUser, cfg.User},
	}

	for _, e := range entries {
		if err := c.SetConfig(e[0], e[1]); err != nil {
			return err
		}
	}

	if cfg.Container != "" {
		return c.SetConfig(KeyContainer, cfg.Container)
	}
	return c.unset(KeyContainer)
}

func (c Client) CurrentBranch(fallbackSHA string) (string, error) {
	if branch, err := c.run("symbolic-ref", "--short", "HEAD"); err == nil && branch != "" {
		return branch, nil
	}

	if sha, err := c.run("rev-parse", "--short", "HEAD"); err == nil && sha != "" {
		return sha, nil
	}

	if len(fallbackSHA) > 7 {
		return fallbackSHA[:7], nil
	}
	if fallbackSHA != "" {
		return fallbackSHA, nil
	}

	return "", errors.New("cannot detect current branch")
}

func (c Client) Branches() ([]string, error) {
	out, err := c.run("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil || out == "" {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(out, "\n") {
		if b := strings.TrimSpace(line); b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}
