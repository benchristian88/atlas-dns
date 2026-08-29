package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/version"
)

const (
	FormatVersion     = 1
	Application       = "atlas-dns"
	magic             = "ATLASDNSBACKUP\n"
	maxManifestBytes  = 64 << 10
	MaxArchiveBytes   = 4 << 30
	maxCredentialSize = 4096
)

type Type string

const (
	Standard Type = "standard"
	Full     Type = "full"
)

type Manifest struct {
	BackupFormatVersion int               `json:"backupFormatVersion"`
	Application         string            `json:"application"`
	ApplicationVersion  string            `json:"applicationVersion"`
	BuildIdentifier     string            `json:"buildIdentifier"`
	DatabaseSchema      int64             `json:"databaseSchemaVersion"`
	CreatedAt           time.Time         `json:"createdAt"`
	Type                Type              `json:"type"`
	IncludedComponents  []string          `json:"includedComponents"`
	ExcludedComponents  []string          `json:"excludedComponents"`
	EntrySHA256         map[string]string `json:"entrySha256"`
	RequiresPassphrase  bool              `json:"requiresPassphrase"`
	SessionsRestored    bool              `json:"sessionsRestored"`
}

type Envelope struct {
	Manifest      Manifest `json:"manifest"`
	PayloadSHA256 string   `json:"payloadSha256"`
	PayloadBytes  int64    `json:"payloadBytes"`
}

type Result struct {
	Path     string
	Manifest Manifest
	Size     int64
}

type SchemaReader interface {
	CurrentSchemaVersion(context.Context) (int64, error)
	RecordAuditEvent(context.Context, domain.AuditEvent) error
}

type Service struct {
	databaseURL string
	key         []byte
	pgDump      string
	schema      SchemaReader
	now         func() time.Time
}

func NewService(databaseURL string, credentialKey []byte, pgDump string, schema SchemaReader) *Service {
	if strings.TrimSpace(pgDump) == "" {
		pgDump = "pg_dump"
	}
	return &Service{databaseURL: databaseURL, key: slices.Clone(credentialKey), pgDump: pgDump, schema: schema, now: time.Now}
}

func (s *Service) Create(ctx context.Context, backupType Type, passphrase string, actor domain.Actor) (Result, error) {
	if backupType != Standard && backupType != Full {
		return Result{}, domain.Validation("type", "must be standard or full")
	}
	if len(passphrase) < 16 || len(passphrase) > 1024 {
		return Result{}, domain.Validation("passphrase", "must contain between 16 and 1024 characters")
	}
	temporary, err := os.MkdirTemp("", "atlas-dns-backup-")
	if err != nil {
		return Result{}, fmt.Errorf("create restricted backup workspace: %w", err)
	}
	if err := os.Chmod(temporary, 0o700); err != nil {
		_ = os.RemoveAll(temporary)
		return Result{}, fmt.Errorf("protect backup workspace: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(temporary)
		}
	}()

	dumpPath := filepath.Join(temporary, "database.dump")
	args := []string{"--format=custom", "--compress=6", "--no-owner", "--no-acl", "--file", dumpPath}
	for _, table := range alwaysExcludedTables {
		args = append(args, "--exclude-table-data="+table)
	}
	if backupType == Standard {
		for _, table := range standardExcludedTables {
			args = append(args, "--exclude-table-data="+table)
		}
	}
	safeDatabaseURL, commandEnvironment, err := protectedDatabaseCommand(s.databaseURL)
	if err != nil {
		return Result{}, err
	}
	args = append(args, safeDatabaseURL)
	command := exec.CommandContext(ctx, s.pgDump, args...)
	command.Env = commandEnvironment
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return Result{}, fmt.Errorf("create PostgreSQL backup: %s", safeToolError(stderr.String()))
	}
	if err := os.Chmod(dumpPath, 0o600); err != nil {
		return Result{}, fmt.Errorf("protect database dump: %w", err)
	}
	dumpInfo, err := os.Stat(dumpPath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect database dump: %w", err)
	}
	if dumpInfo.Size() <= 0 || dumpInfo.Size() > MaxArchiveBytes {
		return Result{}, domain.NewError(domain.ErrorValidation, "database dump is outside supported backup bounds")
	}
	dumpChecksum, err := checksumFile(dumpPath)
	if err != nil {
		return Result{}, err
	}
	schemaVersion, err := s.schema.CurrentSchemaVersion(ctx)
	if err != nil {
		return Result{}, err
	}
	components := []string{"control_plane", "users", "encrypted_credentials", "mfa_encrypted_secrets", "mfa_recovery_hashes", "audit", "lifecycle", "credential_key"}
	excluded := []string{"sessions", "mfa_challenges", "release_cache"}
	if backupType == Full {
		components = append(components, "statistics", "query_log", "dns_probe_history", "ha_operational_history")
	} else {
		excluded = append(excluded, "statistics", "query_log", "dns_probe_history", "ha_operational_history")
	}
	info := version.Current()
	manifest := Manifest{BackupFormatVersion: FormatVersion, Application: Application, ApplicationVersion: info.Version, BuildIdentifier: info.Commit, DatabaseSchema: schemaVersion, CreatedAt: s.now().UTC(), Type: backupType, IncludedComponents: components, ExcludedComponents: excluded, EntrySHA256: map[string]string{"database.dump": dumpChecksum, "credential.key": checksumBytes([]byte(base64.StdEncoding.EncodeToString(s.key)))}, RequiresPassphrase: true, SessionsRestored: false}
	innerManifest, err := json.Marshal(manifest)
	if err != nil {
		return Result{}, fmt.Errorf("encode backup manifest: %w", err)
	}
	plainPath := filepath.Join(temporary, "payload.tar")
	if err := writePayload(plainPath, dumpPath, innerManifest, []byte(base64.StdEncoding.EncodeToString(s.key))); err != nil {
		return Result{}, err
	}
	cipherPath := filepath.Join(temporary, "payload.age")
	if err := encryptPayload(plainPath, cipherPath, passphrase); err != nil {
		return Result{}, err
	}
	_ = os.Remove(plainPath)
	payloadChecksum, err := checksumFile(cipherPath)
	if err != nil {
		return Result{}, err
	}
	payloadInfo, err := os.Stat(cipherPath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect encrypted payload: %w", err)
	}
	envelope := Envelope{Manifest: manifest, PayloadSHA256: payloadChecksum, PayloadBytes: payloadInfo.Size()}
	archivePath := filepath.Join(temporary, fmt.Sprintf("atlas-dns-%s-%s.atlasdnsbackup", backupType, manifest.CreatedAt.Format("20060102T150405Z")))
	if err := writeEnvelope(archivePath, cipherPath, envelope); err != nil {
		return Result{}, err
	}
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect backup archive: %w", err)
	}
	if archiveInfo.Size() > MaxArchiveBytes {
		return Result{}, domain.NewError(domain.ErrorValidation, "backup archive exceeds the supported size")
	}
	event, err := backupEvent(actor, "backup.created", map[string]any{"type": backupType, "formatVersion": FormatVersion, "schemaVersion": schemaVersion, "sizeBytes": archiveInfo.Size()}, manifest.CreatedAt)
	if err != nil {
		return Result{}, err
	}
	if err := s.schema.RecordAuditEvent(ctx, event); err != nil {
		return Result{}, fmt.Errorf("audit backup creation: %w", err)
	}
	cleanup = false
	return Result{Path: archivePath, Manifest: manifest, Size: archiveInfo.Size()}, nil
}

func Cleanup(result Result) { _ = os.RemoveAll(filepath.Dir(result.Path)) }

func ReadEnvelope(reader io.Reader) (Envelope, error) {
	header := make([]byte, len(magic))
	if _, err := io.ReadFull(reader, header); err != nil || string(header) != magic {
		return Envelope{}, domain.NewError(domain.ErrorValidation, "backup archive header is invalid")
	}
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length == 0 || length > maxManifestBytes {
		return Envelope{}, domain.NewError(domain.ErrorValidation, "backup manifest size is invalid")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return Envelope{}, domain.NewError(domain.ErrorValidation, "backup manifest is incomplete")
	}
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decodeOneJSON(decoder, &envelope); err != nil {
		return Envelope{}, domain.NewError(domain.ErrorValidation, "backup manifest is malformed")
	}
	if envelope.Manifest.BackupFormatVersion != FormatVersion || envelope.Manifest.Application != Application || envelope.PayloadBytes <= 0 || envelope.PayloadBytes > MaxArchiveBytes || !validSHA256(envelope.PayloadSHA256) {
		return Envelope{}, domain.NewError(domain.ErrorValidation, "backup format or payload size is unsupported")
	}
	return envelope, nil
}

type PreflightResult struct {
	Manifest Manifest `json:"manifest"`
	Size     int64    `json:"sizeBytes"`
	Valid    bool     `json:"valid"`
}

// Preflight authenticates and validates an archive without mutating PostgreSQL.
// When extractDirectory is non-empty, verified recovery inputs are written only
// beneath that already-private directory using fixed filenames.
func Preflight(path, passphrase, extractDirectory string) (PreflightResult, error) {
	if len(passphrase) < 1 || len(passphrase) > 1024 {
		return PreflightResult{}, domain.Validation("passphrase", "is required and must not exceed 1024 characters")
	}
	info, err := os.Stat(path)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("inspect backup archive: %w", err)
	}
	if info.Size() <= int64(len(magic)+4) || info.Size() > MaxArchiveBytes {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "backup archive size is outside supported bounds")
	}
	file, err := os.Open(path)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer file.Close()
	envelope, err := ReadEnvelope(file)
	if err != nil {
		return PreflightResult{}, err
	}
	payloadStart, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return PreflightResult{}, err
	}
	if info.Size()-payloadStart != envelope.PayloadBytes {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "encrypted backup payload length does not match its manifest")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, envelope.PayloadBytes)); err != nil {
		return PreflightResult{}, fmt.Errorf("verify encrypted payload: %w", err)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), envelope.PayloadSHA256) {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "encrypted backup payload checksum does not match")
	}
	if _, err := file.Seek(payloadStart, io.SeekStart); err != nil {
		return PreflightResult{}, err
	}
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return PreflightResult{}, domain.Validation("passphrase", "cannot be used to decrypt this backup")
	}
	decrypted, err := age.Decrypt(io.LimitReader(file, envelope.PayloadBytes), identity)
	if err != nil {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "backup passphrase is incorrect or encrypted data is corrupt")
	}
	entries, inner, err := validatePayload(decrypted, envelope.Manifest, extractDirectory)
	if err != nil {
		return PreflightResult{}, err
	}
	if !reflect.DeepEqual(inner, envelope.Manifest) {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "authenticated and outer backup manifests do not match")
	}
	if entries["database.dump"] != envelope.Manifest.EntrySHA256["database.dump"] || entries["credential.key"] != envelope.Manifest.EntrySHA256["credential.key"] {
		return PreflightResult{}, domain.NewError(domain.ErrorValidation, "backup entry checksum does not match")
	}
	return PreflightResult{Manifest: envelope.Manifest, Size: info.Size(), Valid: true}, nil
}

func validatePayload(reader io.Reader, expected Manifest, extractDirectory string) (map[string]string, Manifest, error) {
	allowed := map[string]int64{"manifest.json": maxManifestBytes, "credential.key": maxCredentialSize, "database.dump": MaxArchiveBytes}
	seen := map[string]bool{}
	checksums := map[string]string{}
	var inner Manifest
	tarReader := tar.NewReader(reader)
	for count := 0; ; count++ {
		if count >= len(allowed)+1 {
			return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "backup payload contains too many entries")
		}
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "backup payload archive is malformed")
		}
		limit, ok := allowed[header.Name]
		if !ok || seen[header.Name] || header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != header.Name || header.Size < 0 || header.Size > limit {
			return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "backup payload contains an unsupported or unsafe entry")
		}
		seen[header.Name] = true
		limited := io.LimitReader(tarReader, header.Size)
		if header.Name == "manifest.json" {
			data, readErr := io.ReadAll(limited)
			if readErr != nil {
				return nil, Manifest{}, readErr
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decodeOneJSON(decoder, &inner); err != nil {
				return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "authenticated backup manifest is malformed")
			}
			continue
		}
		hasher := sha256.New()
		var target io.Writer = hasher
		var output *os.File
		if extractDirectory != "" {
			if err := os.MkdirAll(extractDirectory, 0o700); err != nil {
				return nil, Manifest{}, err
			}
			if err := os.Chmod(extractDirectory, 0o700); err != nil {
				return nil, Manifest{}, err
			}
			output, err = os.OpenFile(filepath.Join(extractDirectory, header.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, Manifest{}, err
			}
			target = io.MultiWriter(hasher, output)
		}
		_, copyErr := io.Copy(target, limited)
		if output != nil {
			closeErr := output.Close()
			if copyErr == nil {
				copyErr = closeErr
			}
		}
		if copyErr != nil {
			return nil, Manifest{}, copyErr
		}
		checksums[header.Name] = hex.EncodeToString(hasher.Sum(nil))
	}
	for name := range allowed {
		if !seen[name] {
			return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "backup payload is missing a required entry")
		}
	}
	var trailing [1]byte
	if count, err := reader.Read(trailing[:]); count != 0 || err != io.EOF {
		return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "backup payload contains trailing data")
	}
	if inner.BackupFormatVersion != expected.BackupFormatVersion || inner.Application != Application {
		return nil, Manifest{}, domain.NewError(domain.ErrorValidation, "authenticated backup format is unsupported")
	}
	return checksums, inner, nil
}

func writePayload(target, dump string, manifest, key []byte) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create backup payload: %w", err)
	}
	writer := tar.NewWriter(file)
	entries := []struct {
		name string
		data []byte
		path string
	}{{name: "manifest.json", data: manifest}, {name: "credential.key", data: key}, {name: "database.dump", path: dump}}
	for _, entry := range entries {
		var size int64
		if entry.path != "" {
			info, statErr := os.Stat(entry.path)
			if statErr != nil {
				_ = file.Close()
				return statErr
			}
			size = info.Size()
		} else {
			size = int64(len(entry.data))
		}
		if err := writer.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o600, Size: size, ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}); err != nil {
			_ = file.Close()
			return fmt.Errorf("write backup entry header: %w", err)
		}
		if entry.path != "" {
			source, openErr := os.Open(entry.path)
			if openErr != nil {
				_ = file.Close()
				return openErr
			}
			_, err = io.Copy(writer, source)
			_ = source.Close()
		} else {
			_, err = writer.Write(entry.data)
		}
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("write backup entry: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("finish backup payload: %w", err)
	}
	return file.Close()
}

func encryptPayload(source, target, passphrase string) error {
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("create backup encryption recipient: %w", err)
	}
	recipient.SetWorkFactor(18)
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer, err := age.Encrypt(output, recipient)
	if err != nil {
		_ = output.Close()
		return fmt.Errorf("start backup encryption: %w", err)
	}
	if _, err := io.Copy(writer, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("encrypt backup: %w", err)
	}
	if err := writer.Close(); err != nil {
		_ = output.Close()
		return fmt.Errorf("finish backup encryption: %w", err)
	}
	return output.Close()
}

func writeEnvelope(target, payload string, envelope Envelope) error {
	metadata, err := json.Marshal(envelope)
	if err != nil || len(metadata) > maxManifestBytes {
		return fmt.Errorf("encode outer backup manifest")
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer output.Close()
	if _, err := output.Write([]byte(magic)); err != nil {
		return err
	}
	if err := binary.Write(output, binary.BigEndian, uint32(len(metadata))); err != nil {
		return err
	}
	if _, err := output.Write(metadata); err != nil {
		return err
	}
	input, err := os.Open(payload)
	if err != nil {
		return err
	}
	defer input.Close()
	_, err = io.Copy(output, input)
	return err
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func checksumBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func safeToolError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "the PostgreSQL backup command failed"
	}
	if len(value) > 512 {
		value = value[:512]
	}
	// PostgreSQL URLs can contain credentials; never surface tool text containing one.
	if strings.Contains(value, "://") {
		return "the PostgreSQL backup command failed; inspect protected service logs"
	}
	return value
}

func protectedDatabaseCommand(raw string) (string, []string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", nil, fmt.Errorf("database URL is invalid")
	}
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PGPASSWORD=") {
			environment = append(environment, entry)
		}
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			environment = append(environment, "PGPASSWORD="+password)
			parsed.User = url.User(username)
		}
	}
	return parsed.String(), environment, nil
}

func decodeOneJSON(decoder *json.Decoder, target any) error {
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func backupEvent(actor domain.Actor, action string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	event := domain.AuditEvent{ID: id, ActorType: "system", Action: action, ResourceType: "backup", RequestID: actor.RequestID, Metadata: metadata, CreatedAt: at}
	if actor.UserID != "" {
		event.ActorType = "user"
		actorID := actor.UserID
		event.ActorUserID = &actorID
	}
	return event, nil
}

var alwaysExcludedTables = []string{
	"sessions", "mfa_challenges", "upstream_release_cache", "controller_release_cache",
}

var standardExcludedTables = []string{
	"statistics_poll_attempts", "statistics_snapshots", "statistics_buckets",
	"query_ingestion_checkpoints", "query_ingestion_attempts", "query_events", "dns_probe_results", "ha_operational_events", "notification_deliveries",
}
