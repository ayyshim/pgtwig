package mysql

import (
	"errors"
	"fmt"
)

const (
	MaxIdentBytes  = 64
	MaxDBNameBytes = 55 // 55 + "-" + 8-char hash = 64
)

func ValidateDBName(name string) error {
	if name == "" { return errors.New("--db is required") }
	if len(name) > MaxDBNameBytes { return fmt.Errorf("db name too long: %d bytes (max %d)", len(name), MaxDBNameBytes) }
	return nil
}
