package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreflightAuthenticatesManifestAndEntries(t *testing.T) {
	directory := t.TempDir()
	dump := filepath.Join(directory, "database.dump")
	if err := os.WriteFile(dump, []byte("postgres-custom-dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	manifest := Manifest{BackupFormatVersion: FormatVersion, Application: Application, ApplicationVersion: "1.0.0", BuildIdentifier: "test", DatabaseSchema: 12, CreatedAt: time.Unix(1, 0).UTC(), Type: Standard, IncludedComponents: []string{"control_plane"}, EntrySHA256: map[string]string{"database.dump": checksumBytes([]byte("postgres-custom-dump")), "credential.key": checksumBytes(key)}, RequiresPassphrase: true}
	inner, _ := json.Marshal(manifest)
	plain := filepath.Join(directory, "payload.tar")
	if err := writePayload(plain, dump, inner, key); err != nil {
		t.Fatal(err)
	}
	cipher := filepath.Join(directory, "payload.age")
	if err := encryptPayload(plain, cipher, "this is a strong test passphrase"); err != nil {
		t.Fatal(err)
	}
	digest, _ := checksumFile(cipher)
	info, _ := os.Stat(cipher)
	archive := filepath.Join(directory, "test.atlasdnsbackup")
	if err := writeEnvelope(archive, cipher, Envelope{Manifest: manifest, PayloadSHA256: digest, PayloadBytes: info.Size()}); err != nil {
		t.Fatal(err)
	}
	extract := filepath.Join(directory, "extract")
	result, err := Preflight(archive, "this is a strong test passphrase", extract)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Manifest.Type != Standard {
		t.Fatalf("unexpected preflight result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(extract, "database.dump")); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightRejectsWrongPassphraseAndCorruption(t *testing.T) {
	directory := t.TempDir()
	dump := filepath.Join(directory, "database.dump")
	_ = os.WriteFile(dump, []byte("dump"), 0o600)
	key := []byte("key")
	manifest := Manifest{BackupFormatVersion: FormatVersion, Application: Application, ApplicationVersion: "1.0.0", DatabaseSchema: 12, CreatedAt: time.Now().UTC(), Type: Full, EntrySHA256: map[string]string{"database.dump": checksumBytes([]byte("dump")), "credential.key": checksumBytes(key)}}
	inner, _ := json.Marshal(manifest)
	plain := filepath.Join(directory, "payload.tar")
	_ = writePayload(plain, dump, inner, key)
	cipher := filepath.Join(directory, "payload.age")
	_ = encryptPayload(plain, cipher, "correct passphrase value")
	digest, _ := checksumFile(cipher)
	info, _ := os.Stat(cipher)
	archive := filepath.Join(directory, "test.atlasdnsbackup")
	_ = writeEnvelope(archive, cipher, Envelope{Manifest: manifest, PayloadSHA256: digest, PayloadBytes: info.Size()})
	if _, err := Preflight(archive, "wrong passphrase", ""); err == nil {
		t.Fatal("expected wrong passphrase rejection")
	}
	file, _ := os.OpenFile(archive, os.O_WRONLY|os.O_APPEND, 0)
	_, _ = file.Write([]byte("corrupt"))
	_ = file.Close()
	if _, err := Preflight(archive, "correct passphrase value", ""); err == nil {
		t.Fatal("expected corrupted length rejection")
	}
}

func TestPreflightRejectsAuthenticatedTrailingPayloadData(t *testing.T) {
	directory := t.TempDir()
	dump := filepath.Join(directory, "database.dump")
	if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := []byte("key")
	manifest := Manifest{BackupFormatVersion: FormatVersion, Application: Application, ApplicationVersion: "1.0.0", DatabaseSchema: 13, CreatedAt: time.Now().UTC(), Type: Standard, EntrySHA256: map[string]string{"database.dump": checksumBytes([]byte("dump")), "credential.key": checksumBytes(key)}}
	inner, _ := json.Marshal(manifest)
	plain := filepath.Join(directory, "payload.tar")
	if err := writePayload(plain, dump, inner, key); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(plain, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("authenticated trailing data"))
	_ = file.Close()
	cipher := filepath.Join(directory, "payload.age")
	if err := encryptPayload(plain, cipher, "correct passphrase value"); err != nil {
		t.Fatal(err)
	}
	digest, _ := checksumFile(cipher)
	info, _ := os.Stat(cipher)
	archive := filepath.Join(directory, "test.atlasdnsbackup")
	if err := writeEnvelope(archive, cipher, Envelope{Manifest: manifest, PayloadSHA256: digest, PayloadBytes: info.Size()}); err != nil {
		t.Fatal(err)
	}
	if _, err := Preflight(archive, "correct passphrase value", ""); err == nil {
		t.Fatal("expected authenticated trailing payload rejection")
	}
}

func TestCompatibilityRejectsFutureSchemaAndVersion(t *testing.T) {
	if err := ValidateCompatibility(Manifest{Application: "agh-ha-controller", ApplicationVersion: "0.9.2", DatabaseSchema: 12}, "1.0.0", 12); err == nil {
		t.Fatal("expected pre-1.0 application identity rejection")
	}
	if err := ValidateCompatibility(Manifest{Application: Application, ApplicationVersion: "1.1.0", DatabaseSchema: 12}, "1.0.0", 12); err == nil {
		t.Fatal("expected future application rejection")
	}
	if err := ValidateCompatibility(Manifest{Application: Application, ApplicationVersion: "1.0.0", DatabaseSchema: 13}, "1.0.0", 12); err == nil {
		t.Fatal("expected future schema rejection")
	}
}

func TestReadEnvelopeRejectsTrailingJSONAndInvalidChecksum(t *testing.T) {
	for name, metadata := range map[string]string{
		"trailing value": `{"manifest":{"backupFormatVersion":1},"payloadSha256":"` + strings.Repeat("0", 64) + `","payloadBytes":1}{}`,
		"bad checksum":   `{"manifest":{"backupFormatVersion":1},"payloadSha256":"not-a-digest","payloadBytes":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			var archive bytes.Buffer
			archive.WriteString(magic)
			if err := binary.Write(&archive, binary.BigEndian, uint32(len(metadata))); err != nil {
				t.Fatal(err)
			}
			archive.WriteString(metadata)
			if _, err := ReadEnvelope(&archive); err == nil {
				t.Fatal("expected malformed metadata rejection")
			}
		})
	}
}

func TestProtectedDatabaseCommandRemovesPasswordFromArgumentsAndOverridesEnvironment(t *testing.T) {
	t.Setenv("PGPASSWORD", "stale-environment-secret")
	safeURL, environment, err := protectedDatabaseCommand("postgres://operator:database-secret@db.example.test/controller?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(safeURL, "database-secret") || !strings.Contains(safeURL, "operator@") {
		t.Fatalf("unsafe database command URL: %q", safeURL)
	}
	passwordEntries := []string{}
	for _, entry := range environment {
		if strings.HasPrefix(entry, "PGPASSWORD=") {
			passwordEntries = append(passwordEntries, entry)
		}
	}
	if len(passwordEntries) != 1 || passwordEntries[0] != "PGPASSWORD=database-secret" {
		t.Fatalf("unexpected password environment: %#v", passwordEntries)
	}
}

func TestStandardBackupExcludesAllTransientAndOptionalHistoryTables(t *testing.T) {
	for _, required := range []string{"sessions", "controller_release_cache", "statistics_snapshots", "query_events", "dns_probe_results", "ha_operational_events"} {
		if !contains(optionalTables, required) {
			t.Fatalf("standard backup does not exclude %q", required)
		}
	}
	for _, requiredState := range []string{"system_settings", "notification_policy"} {
		if contains(optionalTables, requiredState) {
			t.Fatalf("standard backup incorrectly excludes required state %q", requiredState)
		}
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
