package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSONAtomic encodes value with deterministic indentation and replaces
// path only after the complete payload is flushed to disk.
func WriteJSONAtomic(path string, value any, permission os.FileMode) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	payload = append(payload, '\n')
	return WriteFileAtomic(path, payload, permission)
}

func WriteFileAtomic(path string, payload []byte, permission os.FileMode) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".bruno-write-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err = temporary.Chmod(permission); err != nil {
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err = temporary.Write(payload); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err = temporary.Sync(); err != nil {
		return fmt.Errorf("flush temporary file: %w", err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err = replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}
	if err = syncDirectory(directory); err != nil {
		return fmt.Errorf("flush parent directory: %w", err)
	}
	return nil
}
