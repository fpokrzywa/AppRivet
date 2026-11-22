package admission

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"reflect"
	"time"
)

type MaintenanceReport struct {
	Mode                   string `json:"mode"`
	Action                 string `json:"action"`
	AuthenticationVerified bool   `json:"authentication_verified"`
	ExecutionAuthorized    bool   `json:"execution_authorized"`
	Changed                bool   `json:"changed"`
	Format                 int    `json:"format"`
	Generation             int    `json:"generation"`
	JournalSequence        int    `json:"journal_sequence"`
	JournalSHA256          string `json:"journal_sha256"`
	RetainedReservations   int    `json:"retained_reservations"`
	ManifestSHA256         string `json:"manifest_sha256"`
	PreparedManifest       string `json:"prepared_manifest,omitempty"`
	CleanupPending         bool   `json:"cleanup_pending"`
}

func (s Store) Migrate(ctx context.Context) (MaintenanceReport, error) {
	return s.maintain(ctx, "migrate")
}
func (s Store) Compact(ctx context.Context) (MaintenanceReport, error) {
	return s.maintain(ctx, "compact")
}
func (s Store) maintain(ctx context.Context, action string) (MaintenanceReport, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return MaintenanceReport{}, err
	}
	defer tx.close()
	return tx.maintain(ctx, s.now(), action)
}

type stagedFile struct {
	name string
	info os.FileInfo
}

func (t *transaction) stageFile(name string, b []byte) (stagedFile, error) {
	var f journalFile
	var err error
	if t.openStage == nil {
		f, err = t.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	} else {
		f, err = t.openStage(name)
	}
	if err != nil {
		return stagedFile{}, stateIO()
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return stagedFile{}, stateIO()
	}
	stage := stagedFile{name, info}
	n, err := f.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return stage, stateIO()
	}
	return stage, nil
}
func removeOwned(root *os.Root, s stagedFile) bool {
	info, err := root.Lstat(s.name)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, s.info) {
		return false
	}
	return root.Remove(s.name) == nil
}
func (t *transaction) maintain(ctx context.Context, now time.Time, action string) (MaintenanceReport, error) {
	if err := cancelled(ctx); err != nil {
		return MaintenanceReport{}, err
	}
	if now.IsZero() || now.Before(t.state.at) {
		return MaintenanceReport{}, failure("clock_rollback", "clock precedes persistent watermark; maintenance refused")
	}
	result := MaintenanceReport{Mode: "local_journal_maintenance", Action: action, Format: 2, JournalSequence: t.state.sequence, JournalSHA256: t.state.hash, RetainedReservations: len(t.state.jobs)}
	if action == "compact" && t.manifest == nil {
		return MaintenanceReport{}, failure("migration_required", "migrate the legacy journal before compaction")
	}
	if t.manifest != nil {
		result.Generation = t.manifest.Generation
		result.ManifestSHA256 = t.manifest.Hash
		if action == "migrate" || t.size == 0 {
			return result, nil
		}
		if t.manifest.Generation >= maxSegments {
			return MaintenanceReport{}, failure("state_full", "immutable segment limit reached; no history was discarded")
		}
	}
	if !t.identitiesUnchanged() {
		return MaintenanceReport{}, stateIO()
	}
	if _, err := t.file.Seek(0, io.SeekStart); err != nil {
		return MaintenanceReport{}, stateIO()
	}
	original, err := io.ReadAll(io.LimitReader(t.file, maxJournalBytes+1))
	if err != nil || int64(len(original)) != t.size {
		return MaintenanceReport{}, stateIO()
	}
	var packed bytes.Buffer
	z, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	if _, err = z.Write(original); err != nil {
		return MaintenanceReport{}, stateIO()
	}
	if err = z.Close(); err != nil {
		return MaintenanceReport{}, stateIO()
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return MaintenanceReport{}, stateIO()
	}
	id := hex.EncodeToString(random[:])
	d := historySegment{Name: "segment-" + id + ".jsonl.gz", Bytes: t.size, Events: t.activeEvents, SHA256: sum(original), CompressedBytes: int64(packed.Len()), CompressedSHA256: sum(packed.Bytes())}
	m := journalManifest{SchemaVersion: 2, StoreID: id, Generation: 1, Previous: sum(original), SealedSequence: t.state.sequence, Head: t.state.hash, At: stamp(t.state.at), Segments: []historySegment{d}, Active: "active-" + id + ".jsonl"}
	if t.manifest != nil {
		m.StoreID = t.manifest.StoreID
		m.Generation = t.manifest.Generation + 1
		m.Previous = t.manifest.Hash
		m.Segments = append(append([]historySegment(nil), t.manifest.Segments...), d)
	}
	body := manifestBytes(m)
	m, err = parseManifest(body)
	if err != nil {
		return MaintenanceReport{}, err
	}
	preparedName := "manifest-" + id + ".json"
	source := original
	if t.manifest != nil {
		source = manifestBytes(*t.manifest)
	}
	intent := publicationIntent{SchemaVersion: 1, OperationID: id, PreparedManifest: preparedName, ManifestSHA256: m.Hash, SourceFile: "publication-source-" + id + ".bin", SourceSHA256: sum(source), SourceActiveName: t.fileName, SourceActiveSHA256: sum(original), SourceActiveBytes: t.size, Sequence: t.state.sequence, Head: t.state.hash, RecordedAt: stamp(t.state.at)}
	intentBody := intentBytes(intent)
	var staged []stagedFile
	publicationStarted := false
	defer func() {
		if !publicationStarted {
			for _, f := range staged {
				removeOwned(t.root, f)
			}
		}
	}()
	for _, item := range []struct {
		name string
		data []byte
	}{{d.Name, packed.Bytes()}, {m.Active, nil}, {preparedName, body}, {intent.SourceFile, source}, {"publication-" + id + ".intent.json", intentBody}} {
		if err = cancelled(ctx); err != nil {
			return MaintenanceReport{}, err
		}
		f, stageErr := t.stageFile(item.name, item.data)
		if f.info != nil {
			staged = append(staged, f)
		}
		if stageErr != nil {
			return MaintenanceReport{}, stageErr
		}
	}
	// Reopen and fully verify staged bytes/history before changing the authority.
	verified := &transaction{root: t.root}
	err = verified.loadManifest(ctx, m)
	if verified.file != nil {
		verified.file.Close()
	}
	if err != nil {
		return MaintenanceReport{}, err
	}
	if !reflect.DeepEqual(t.state, verified.state) {
		return MaintenanceReport{}, corrupt()
	}
	if err = syncStoreDirectory(t.root); err != nil {
		return MaintenanceReport{}, stateIO()
	}
	if !t.identitiesUnchanged() {
		return MaintenanceReport{}, stateIO()
	}
	if err = cancelled(ctx); err != nil {
		return MaintenanceReport{}, err
	}
	control := t.file
	if t.control != nil {
		control = t.control
	}
	oldInfo, err := t.file.Stat()
	if err != nil {
		return MaintenanceReport{}, stateIO()
	}
	pending, err := t.stageEvidence(pendingName, intentBody)
	if err != nil {
		return MaintenanceReport{}, err
	}
	publicationStarted = true // A complete marker is visible. Retain its evidence.
	if syncStoreDirectory(t.root) != nil {
		return MaintenanceReport{}, publicationUncertain()
	}
	if err = publishManifest(control, body); err != nil {
		return MaintenanceReport{}, err
	}
	// Preserve the original active file until the publication marker is cleared.
	if !removeOwned(t.root, pending) || syncStoreDirectory(t.root) != nil {
		return MaintenanceReport{}, publicationUncertain()
	}
	result.Changed = true
	result.Generation = m.Generation
	result.ManifestSHA256 = m.Hash
	result.PreparedManifest = preparedName
	if t.manifest != nil {
		// The compressed, replay-verified copy is authoritative before raw removal.
		if t.file.Close() != nil {
			result.CleanupPending = true
		} else {
			result.CleanupPending = !removeOwned(t.root, stagedFile{t.fileName, oldInfo})
		}
		t.file = nil
	}
	if syncStoreDirectory(t.root) != nil {
		result.CleanupPending = true
	}
	return result, nil
}

func publishManifest(control journalFile, body []byte) error {
	uncertain := publicationUncertain
	if _, err := control.Seek(0, io.SeekStart); err != nil {
		return uncertain()
	}
	if err := control.Truncate(0); err != nil {
		return uncertain()
	}
	n, err := control.Write(body)
	if err != nil || n != len(body) {
		return uncertain()
	}
	if err = control.Sync(); err != nil {
		return uncertain()
	}
	return nil
}

func publicationUncertain() error {
	return failure("commit_uncertain", "manifest publication is uncertain; retain all evidence and inspect repair status before retrying")
}
