package mysql

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

func QuoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func QuoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (c Client) Exists(db string) (bool, error) {
	out, err := c.execSQL(fmt.Sprintf("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = %s;\n", QuoteLiteral(db)))
	return out == "1", err
}

func (c Client) List() (map[string]bool, error) {
	out, err := c.execSQL("SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME NOT IN ('mysql','information_schema','performance_schema','sys') ORDER BY SCHEMA_NAME;\n")
	if err != nil { return nil, err }
	dbs := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" { dbs[name] = true }
	}
	return dbs, nil
}

func (c Client) ListPrefix(prefix string) ([]string, error) {
	dbs, err := c.List()
	if err != nil { return nil, err }
	var result []string
	for db := range dbs { if strings.HasPrefix(db, prefix) { result = append(result, db) } }
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j] < result[j-1]; j-- { result[j], result[j-1] = result[j-1], result[j] }
	}
	return result, nil
}

func (c Client) Terminate(db string) error {
	out, err := c.execSQL(fmt.Sprintf("SELECT ID FROM information_schema.processlist WHERE DB = %s AND ID <> CONNECTION_ID();\n", QuoteLiteral(db)))
	if err != nil { return err }
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		if id != "" { if _, err := c.execSQL("KILL " + id + ";\n"); err != nil { return err } }
	}
	return nil
}

func (c Client) Drop(db string) error {
	if err := c.Terminate(db); err != nil { return err }
	_, err := c.execSQL(fmt.Sprintf("DROP DATABASE IF EXISTS %s;\n", QuoteIdent(db)))
	return err
}

func (c Client) Rename(from, to string) error {
	if err := c.Terminate(from); err != nil { return err }
	if _, err := c.execSQL(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;\n", QuoteIdent(to))); err != nil { return err }
	out, err := c.execSQL(fmt.Sprintf("SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME;\n", QuoteLiteral(from)))
	if err != nil { return err }
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" { continue }
		if _, err := c.execSQL(fmt.Sprintf("RENAME TABLE %s.%s TO %s.%s;\n", QuoteIdent(from), QuoteIdent(name), QuoteIdent(to), QuoteIdent(name))); err != nil { return err }
	}
	_, err = c.execSQL(fmt.Sprintf("DROP DATABASE IF EXISTS %s;\n", QuoteIdent(from)))
	return err
}

func (c Client) Clone(target, template string) error {
	if err := c.Terminate(template); err != nil { return err }
	if _, err := c.execSQL(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;\n", QuoteIdent(target))); err != nil { return err }
	out, err := c.execSQL(fmt.Sprintf("SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME;\n", QuoteLiteral(template)))
	if err != nil { return err }
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" { continue }
		if _, err := c.execSQL(fmt.Sprintf("CREATE TABLE %s.%s LIKE %s.%s;\n", QuoteIdent(target), QuoteIdent(name), QuoteIdent(template), QuoteIdent(name))); err != nil { return err }
		if _, err := c.execSQL(fmt.Sprintf("INSERT INTO %s.%s SELECT * FROM %s.%s;\n", QuoteIdent(target), QuoteIdent(name), QuoteIdent(template), QuoteIdent(name))); err != nil { return err }
	}
	return nil
}
