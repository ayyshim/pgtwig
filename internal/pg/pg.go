package pg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const EnvPassword = "PGTWIG_PGPASSWORD"

type Client struct {
	User      string
	Container string
	Exec      func(sql string) (string, error)
}

// BuildCommand never passes -t to docker exec so git hooks run without a TTY.
func BuildCommand(container, user, password string) (bin string, args []string, extraEnv []string) {
	psql := []string{"-X"}
	if user != "" {
		psql = append(psql, "-U", user)
	}
	psql = append(psql, "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-Atq")

	if container != "" {
		docker := []string{"exec", "-i"}
		if password != "" {
			docker = append(docker, "-e", "PGPASSWORD="+password)
		}
		docker = append(docker, container, "psql")
		return "docker", append(docker, psql...), nil
	}

	if password != "" {
		extraEnv = []string{"PGPASSWORD=" + password}
	}
	return "psql", psql, extraEnv
}

func (c Client) execSQL(sql string) (string, error) {
	if c.Exec != nil {
		return c.Exec(sql)
	}

	bin, args, extraEnv := BuildCommand(c.Container, c.User, os.Getenv(EnvPassword))

	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(sql)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}

	return strings.TrimSpace(out.String()), nil
}

func ContainerRunning(name string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name)

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return false, fmt.Errorf("%s", msg)
		}
		return false, err
	}

	return strings.TrimSpace(out.String()) == "true", nil
}

func (c Client) Exists(db string) (bool, error) {
	sql := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = %s;\n", QuoteLiteral(db))
	out, err := c.execSQL(sql)
	return out == "1", err
}

func (c Client) List() (map[string]bool, error) {
	out, err := c.execSQL("SELECT datname FROM pg_database;\n")
	if err != nil {
		return nil, err
	}

	dbs := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			dbs[name] = true
		}
	}
	return dbs, nil
}

func (c Client) ListPrefix(prefix string) ([]string, error) {
	sql := fmt.Sprintf("SELECT datname FROM pg_database WHERE strpos(datname, %s) = 1 ORDER BY datname;\n", QuoteLiteral(prefix))
	out, err := c.execSQL(sql)
	if err != nil || out == "" {
		return nil, err
	}

	var dbs []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); strings.HasPrefix(name, prefix) {
			dbs = append(dbs, name)
		}
	}
	return dbs, nil
}

func (c Client) Terminate(db string) error {
	sql := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid();\n",
		QuoteLiteral(db),
	)
	_, err := c.execSQL(sql)
	return err
}

func (c Client) Drop(db string) error {
	if err := c.Terminate(db); err != nil {
		return err
	}
	_, err := c.execSQL(fmt.Sprintf("DROP DATABASE IF EXISTS %s;\n", QuoteIdent(db)))
	return err
}

func (c Client) Rename(from, to string) error {
	if err := c.Terminate(from); err != nil {
		return err
	}
	_, err := c.execSQL(fmt.Sprintf("ALTER DATABASE %s RENAME TO %s;\n", QuoteIdent(from), QuoteIdent(to)))
	return err
}

func (c Client) Clone(target, template string) error {
	if err := c.Terminate(template); err != nil {
		return err
	}

	fast := fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s STRATEGY = FILE_COPY;\n", QuoteIdent(target), QuoteIdent(template))
	_, err := c.execSQL(fast)
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "strategy") && !strings.Contains(msg, "syntax error") {
		return err
	}

	fallback := fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s;\n", QuoteIdent(target), QuoteIdent(template))
	_, err = c.execSQL(fallback)
	return err
}
