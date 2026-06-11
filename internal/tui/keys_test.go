package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func keysContain(s []string, v string) bool {
	return slices.Contains(s, v)
}

func TestLoadKeyMapOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(path, []byte(`{"deleteTicket":["X","delete"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	km := LoadKeyMap(path)
	if got := km.DeleteTicket.Keys(); !keysContain(got, "X") || !keysContain(got, "delete") {
		t.Fatalf("override not applied: %v", got)
	}
	// Unspecified bindings keep their defaults.
	if got := km.NewTicket.Keys(); !keysContain(got, "n") {
		t.Fatalf("default should remain: %v", got)
	}

	// A missing file yields all defaults.
	def := LoadKeyMap(filepath.Join(dir, "absent.json"))
	if got := def.DeleteTicket.Keys(); !keysContain(got, "x") {
		t.Fatalf("missing file should give defaults: %v", got)
	}
}
