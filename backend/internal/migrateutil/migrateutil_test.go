package migrateutil

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSourceURL(t *testing.T) {
	dir := t.TempDir()
	got, err := FileSourceURL(dir)
	if err != nil {
		t.Fatalf("FileSourceURL: %v", err)
	}
	if !strings.HasPrefix(got, "file://") {
		t.Fatalf("expected file:// prefix, got %q", got)
	}
	if strings.Contains(got, ":///") {
		t.Fatalf("Windows-safe URL must not use file:///, got %q", got)
	}
	abs, _ := filepath.Abs(dir)
	if !strings.Contains(got, filepath.ToSlash(abs)) {
		t.Fatalf("URL should contain absolute migrations dir, got %q", got)
	}
}
