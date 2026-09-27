package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateRenamesLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "cloud-torrent.json")
	os.WriteFile(legacy, []byte(`{"IncomingPort":50007}`), 0600)

	moved, err := migrateLegacyConfig(filepath.Join(dir, "cloud-fetch.json"))
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v, want true/nil", moved, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "cloud-fetch.json"))
	if err != nil || string(b) != `{"IncomingPort":50007}` {
		t.Fatalf("settings not carried over: %q %v", b, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy file still present, so the next start would be ambiguous")
	}
}

func TestMigrateNeverOverwritesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "cloud-torrent.json"), []byte("old"), 0600)
	os.WriteFile(filepath.Join(dir, "cloud-fetch.json"), []byte("new"), 0600)

	moved, err := migrateLegacyConfig(filepath.Join(dir, "cloud-fetch.json"))
	if err != nil || moved {
		t.Fatalf("moved=%v err=%v, want false/nil", moved, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "cloud-fetch.json")); string(b) != "new" {
		t.Fatalf("existing config overwritten with %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "cloud-torrent.json")); string(b) != "old" {
		t.Fatal("legacy file touched although it was not migrated")
	}
}

func TestMigrateDoesNothingWithoutLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	moved, err := migrateLegacyConfig(filepath.Join(dir, "cloud-fetch.json"))
	if err != nil || moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("migration created %d files", len(entries))
	}
}

func TestMigrateLeavesCustomConfigPathAlone(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "cloud-torrent.json")
	os.WriteFile(legacy, []byte("old"), 0600)

	moved, err := migrateLegacyConfig(filepath.Join(dir, "settings.json"))
	if err != nil || moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy file moved for a custom --config-path")
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("custom config path was created from the legacy file")
	}
}
