package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"
)

const appendPendingName = "append.pending"
const maxAppendIntentBytes = 8 << 10

type appendIntent struct {
	SchemaVersion  int    `json:"schema_version"`
	Active         string `json:"active"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Offset         int64  `json:"offset"`
	Record         []byte `json:"record"`
	Hash           string `json:"hash"`
}

func appendIntentBytes(intent appendIntent) []byte {
	intent.Hash = ""
	body, _ := json.Marshal(intent)
	intent.Hash = sum(body)
	body, _ = json.Marshal(intent)
	return append(body, '\n')
}

func parseAppendIntent(body []byte) (appendIntent, event, error) {
	var intent appendIntent
	var e event
	if len(body) > maxAppendIntentBytes || json.Unmarshal(body, &intent) != nil || !bytes.Equal(body, appendIntentBytes(intent)) || intent.SchemaVersion != 1 {
		return intent, e, corrupt()
	}
	legacy := intent.Active == "journal.jsonl" && intent.ManifestSHA256 == ""
	segmented := activeName.MatchString(intent.Active) && digest.MatchString(intent.ManifestSHA256)
	if (!legacy && !segmented) || intent.Offset < 0 || intent.Offset > maxJournalBytes || len(intent.Record) < 2 || len(intent.Record) > maxEventBytes+1 || intent.Offset+int64(len(intent.Record)) > maxJournalBytes || intent.Record[len(intent.Record)-1] != '\n' {
		return intent, e, corrupt()
	}
	line := intent.Record[:len(intent.Record)-1]
	if json.Unmarshal(line, &e) != nil {
		return intent, e, corrupt()
	}
	canonical, _ := json.Marshal(e)
	if !bytes.Equal(canonical, line) {
		return intent, e, corrupt()
	}
	return intent, e, nil
}

type AppendRepairExpect struct {
	IntentSHA256 string
	ActiveSHA256 string
}

type AppendRepairReport struct {
	Mode                   string `json:"mode"`
	AuthenticationVerified bool   `json:"authentication_verified"`
	ExecutionAuthorized    bool   `json:"execution_authorized"`
	Changed                bool   `json:"changed"`
	IntentSHA256           string `json:"intent_sha256"`
	ActiveSHA256           string `json:"active_sha256"`
	JournalSHA256          string `json:"journal_sha256"`
	JournalSequence        int    `json:"journal_sequence"`
	EventKind              string `json:"event_kind"`
	PresentBytes           int    `json:"present_bytes"`
	EventBytes             int    `json:"event_bytes"`
	BytesWritten           int    `json:"bytes_written"`
	BytesReasserted        int    `json:"bytes_reasserted"`
	RecordedAt             string `json:"recorded_at"`
	ObservedAt             string `json:"observed_at"`
	ClockRollback          bool   `json:"clock_rollback"`
}

type appendProof struct {
	report     AppendRepairReport
	marker     stagedFile
	markerBody []byte
	record     []byte
	activeSize int64
}

func (s Store) InspectAppendRepair(ctx context.Context) (AppendRepairReport, error) {
	tx, err := s.openLocked(ctx)
	if err != nil {
		return AppendRepairReport{}, err
	}
	defer tx.close()
	proof, err := tx.proveAppend(ctx, s.now())
	return proof.report, err
}

func (s Store) RepairAppend(ctx context.Context, expect AppendRepairExpect) (AppendRepairReport, error) {
	if !digest.MatchString(expect.IntentSHA256) || !digest.MatchString(expect.ActiveSHA256) {
		return AppendRepairReport{}, failure("invalid_arguments", "append repair requires the intent and active file hashes from inspection")
	}
	tx, err := s.openLocked(ctx)
	if err != nil {
		return AppendRepairReport{}, err
	}
	defer tx.close()
	proof, err := tx.proveAppend(ctx, s.now())
	if err != nil {
		return AppendRepairReport{}, err
	}
	return tx.repairAppend(ctx, proof, expect)
}

func (t *transaction) proveAppend(ctx context.Context, now time.Time) (appendProof, error) {
	var proof appendProof
	if err := requireNoPending(t.root, pendingName, "publication_pending", "manifest publication requires repair inspection"); err != nil {
		return proof, err
	}
	if _, err := t.root.Lstat(appendPendingName); os.IsNotExist(err) {
		return proof, failure("append_repair_unavailable", "no recorded append intent; keep damaged journal bytes intact")
	}
	body, marker, err := readEvidence(t.root, appendPendingName, maxAppendIntentBytes)
	if err != nil {
		return proof, err
	}
	intent, e, err := parseAppendIntent(body)
	if err != nil {
		return proof, err
	}
	active, err := t.selectJournal(ctx)
	if err != nil {
		return proof, err
	}
	if t.fileName != intent.Active || t.activeManifestHash() != intent.ManifestSHA256 || intent.Offset > int64(len(active)) {
		return proof, repairRefused()
	}
	tail := active[intent.Offset:]
	if !bytes.HasPrefix(intent.Record, tail) {
		return proof, repairRefused()
	}
	// An empty legacy prefix can only accept the initial event. The same
	// transition validator used by normal writes enforces this and the chain.
	if err = t.replayActive(active[:intent.Offset], true); err != nil {
		return proof, err
	}
	if t.activeEvents >= maxEvents || t.historyBytes+int64(len(intent.Record)) > maxHistoryBytes {
		return proof, repairRefused()
	}
	if err = t.state.apply(e); err != nil {
		return proof, err
	}
	if now.IsZero() {
		return proof, failure("invalid_arguments", "clock must be set")
	}
	if !t.identitiesUnchanged() {
		return proof, stateIO()
	}
	if err = cancelled(ctx); err != nil {
		return proof, err
	}
	proof.marker = marker
	proof.markerBody = body
	proof.record = intent.Record
	proof.activeSize = int64(len(active))
	proof.report = AppendRepairReport{
		Mode: "local_append_repair_inspection", IntentSHA256: sum(body), ActiveSHA256: sum(active),
		JournalSHA256: t.state.hash, JournalSequence: t.state.sequence, EventKind: e.Kind,
		PresentBytes: len(tail), EventBytes: len(intent.Record), RecordedAt: stamp(t.state.at),
		ObservedAt: stamp(now), ClockRollback: now.Before(t.state.at),
	}
	return proof, nil
}

func (t *transaction) repairAppend(ctx context.Context, proof appendProof, expect AppendRepairExpect) (AppendRepairReport, error) {
	r := proof.report
	if r.IntentSHA256 != expect.IntentSHA256 || r.ActiveSHA256 != expect.ActiveSHA256 {
		return AppendRepairReport{}, failure("repair_changed", "append evidence changed; inspect again")
	}
	if r.ClockRollback {
		return AppendRepairReport{}, failure("clock_rollback", "clock precedes recorded append; repair refused")
	}
	if err := cancelled(ctx); err != nil {
		return AppendRepairReport{}, err
	}
	if err := t.syncEvidence(proof.marker, proof.markerBody); err != nil {
		return AppendRepairReport{}, err
	}
	if syncStoreDirectory(t.root) != nil || !t.identitiesUnchanged() {
		return AppendRepairReport{}, stateIO()
	}
	if end, err := t.file.Seek(0, io.SeekEnd); err != nil || end != proof.activeSize {
		return AppendRepairReport{}, stateIO()
	}
	// fsync can report writeback failure even though the bytes remain visible
	// in cache. A later descriptor can miss that error and have nothing to flush.
	// Reassert the exact event within its verified range before syncing again.
	start := proof.activeSize - int64(r.PresentBytes)
	if offset, err := t.file.Seek(start, io.SeekStart); err != nil || offset != start {
		return AppendRepairReport{}, stateIO()
	}
	if err := cancelled(ctx); err != nil {
		return AppendRepairReport{}, err
	}
	if err := t.finishAppend(proof.marker, proof.record); err != nil {
		return AppendRepairReport{}, err
	}
	r.Mode = "local_append_repair"
	r.Changed = true
	r.BytesWritten = len(proof.record) - r.PresentBytes
	r.BytesReasserted = r.PresentBytes
	return r, nil
}

func (t *transaction) activeManifestHash() string {
	if t.manifest != nil {
		return t.manifest.Hash
	}
	return ""
}

func (t *transaction) finishAppend(marker stagedFile, record []byte) error {
	n, err := t.file.Write(record)
	if err != nil || n != len(record) {
		return appendUncertain()
	}
	if t.file.Sync() != nil || !removeOwned(t.root, marker) || syncStoreDirectory(t.root) != nil {
		return appendUncertain()
	}
	return nil
}

func appendUncertain() error {
	return failure("commit_uncertain", "journal append is uncertain; retain state and inspect append repair before retrying")
}
