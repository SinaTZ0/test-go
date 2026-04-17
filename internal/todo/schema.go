package todo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// LoadSchemaSQL reads the shared todo schema used by both sqlc and migrations.
func LoadSchemaSQL() ([]byte, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("locate todo schema file")
	}

	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "sql", "schema", "todos.sql"))
	return os.ReadFile(schemaPath)
}
