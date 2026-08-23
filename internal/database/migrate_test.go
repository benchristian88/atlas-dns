package database

import "testing"

func TestEmbeddedMigrationChainIncludesV102NotificationHistory(t *testing.T) {
	items, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 15 {
		t.Fatalf("migration count = %d, want 15", len(items))
	}
	for index, item := range items {
		wantVersion := int64(index + 1)
		if item.version != wantVersion {
			t.Fatalf("migration %d version = %d, want %d", index, item.version, wantVersion)
		}
		if item.name == "" || item.up == "" || item.down == "" || len(item.checksum) != 64 {
			t.Fatalf("migration %06d is incomplete: %#v", item.version, item)
		}
	}
	if LatestSchemaVersion() != 15 {
		t.Fatalf("latest schema version = %d, want 15", LatestSchemaVersion())
	}
}
