package mysql

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

const EnvPassword = "PGTWIG_MYSQL_PASSWORD"

type Client struct {
	User      string
	Container string
	Exec      func(sql string) (string, error)
}

func BuildCommand(container, user, password string) (bin string, args []string, extraEnv []string) {
	mysql := []string{"--batch", "--skip-column-names", "--no-auto-rehash"}
	if user != "" {
		mysql = append(mysql, "--user="+user)
	}

	if container != "" {
		docker := []string{"exec", "-i"}
		if password != "" {
			docker = append(docker, "-e", "MYSQL_PWD="+password)
		}
		docker = append(docker, container, "mysql")
		return "docker", append(docker, mysql...), nil
	}

	if password != "" {
		extraEnv = []string{"MYSQL_PWD=" + password}
	}
	return "mysql", mysql, extraEnv
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

func QuoteIdent(s string) string   { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func QuoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (c Client) Exists(db string) (bool, error) {
	sql := fmt.Sprintf("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = %s;\n", QuoteLiteral(db))
	out, err := c.execSQL(sql)
	return out == "1", err
}

func (c Client) List() (map[string]bool, error) {
	out, err := c.execSQL("SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('mysql','information_schema','performance_schema','sys') ORDER BY SCHEMA_NAME;\n")
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
	dbs, err := c.List()
	if err != nil {
		return nil, err
	}

	var result []string
	for db := range dbs {
		if strings.HasPrefix(db, prefix) {
			result = append(result, db)
		}
	}
	slices.Sort(result)
	return result, nil
}

func (c Client) listTables(db string) ([]string, error) {
	sql := fmt.Sprintf(
		"SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME;\n",
		QuoteLiteral(db),
	)
	out, err := c.execSQL(sql)
	if err != nil || out == "" {
		return nil, err
	}

	var tables []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			tables = append(tables, name)
		}
	}
	return tables, nil
}

func (c Client) Terminate(db string) error {
	sql := fmt.Sprintf("SELECT ID FROM information_schema.processlist WHERE DB = %s AND ID <> CONNECTION_ID();\n", QuoteLiteral(db))
	out, err := c.execSQL(sql)
	if err != nil || out == "" {
		return err
	}

	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if id := strings.TrimSpace(line); id != "" {
			fmt.Fprintf(&b, "KILL %s;\n", id)
		}
	}
	if b.Len() == 0 {
		return nil
	}

	_, err = c.execSQL(b.String())
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

	tables, err := c.listTables(from)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CREATE DATABASE IF NOT EXISTS %s;\n", QuoteIdent(to))

	if len(tables) > 0 {
		pairs := make([]string, 0, len(tables))
		fromQ, toQ := QuoteIdent(from), QuoteIdent(to)
		for _, t := range tables {
			tq := QuoteIdent(t)
			pairs = append(pairs, fmt.Sprintf("%s.%s TO %s.%s", fromQ, tq, toQ, tq))
		}
		fmt.Fprintf(&b, "RENAME TABLE %s;\n", strings.Join(pairs, ", "))
	}

	fmt.Fprintf(&b, "DROP DATABASE IF EXISTS %s;\n", QuoteIdent(from))

	_, err = c.execSQL(b.String())
	return err
}

func (c Client) Clone(target, template string) error {
	if err := c.Terminate(template); err != nil {
		return err
	}

	tables, err := c.listTables(template)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("SET SESSION foreign_key_checks = 0;\n")
	b.WriteString("SET SESSION unique_checks = 0;\n")
	fmt.Fprintf(&b, "CREATE DATABASE IF NOT EXISTS %s;\n", QuoteIdent(target))

	if len(tables) > 0 {
		targetQ, tmplQ := QuoteIdent(target), QuoteIdent(template)
		for _, t := range tables {
			tq := QuoteIdent(t)
			fmt.Fprintf(&b, "CREATE TABLE %s.%s LIKE %s.%s;\n", targetQ, tq, tmplQ, tq)
		}

		b.WriteString("SET SESSION autocommit = 0;\n")
		for _, t := range tables {
			tq := QuoteIdent(t)
			fmt.Fprintf(&b, "INSERT INTO %s.%s SELECT * FROM %s.%s;\n", targetQ, tq, tmplQ, tq)
		}
		b.WriteString("COMMIT;\n")
	}

	_, err = c.execSQL(b.String())
	return err
}
