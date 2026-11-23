package pg

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	MaxIdentBytes  = 63
	MaxDBNameBytes = 54 // 54 + "-" + 8-char hash = 63
)

func HashLen(dbName string) int {
	n := (MaxIdentBytes - 1) - len(dbName)
	if n > 40 {
		return 40
	}
	if n < 8 {
		return 8
	}
	return n
}

func BranchHash(dbName, branch string) string {
	sum := sha1.Sum([]byte(branch))
	return hex.EncodeToString(sum[:])[:HashLen(dbName)]
}

func BranchDBName(dbName, branch string) string {
	return dbName + "-" + BranchHash(dbName, branch)
}

func ValidateDBName(name string) error {
	if name == "" {
		return errors.New("--db is required")
	}
	if len(name) > MaxDBNameBytes {
		return fmt.Errorf("db name too long: %d bytes (max %d)", len(name), MaxDBNameBytes)
	}
	return nil
}

func QuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func QuoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
