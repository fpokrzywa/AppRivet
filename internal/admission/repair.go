package admission

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
)

type RepairExpect struct {
	IntentSHA256  string
	ControlSHA256 string
	HistorySHA256 string
	Sequence      int
}
type RepairReport struct {
	Mode                   string `json:"mode"`
	AuthenticationVerified bool   `json:"authentication_verified"`
	ExecutionAuthorized    bool   `json:"execution_authorized"`
	Changed                bool   `json:"changed"`
	ControlRewritten       bool   `json:"control_rewritten"`
	IntentSHA256           string `json:"intent_sha256"`
	ControlSHA256          string `json:"control_sha256"`
	ManifestSHA256         string `json:"manifest_sha256"`
	JournalSequence        int    `json:"journal_sequence"`
	JournalSHA256          string `json:"journal_sha256"`
	RecordedAt             string `json:"recorded_at"`
	ObservedAt             string `json:"observed_at"`
	ClockRollback          bool   `json:"clock_rollback"`
	ControlState           string `json:"control_state"`
	PreparedManifest       string `json:"prepared_manifest"`
	RetainedReservations   int    `json:"retained_reservations"`
	BeforeImage            string `json:"before_image,omitempty"`
}
type repairProof struct {
	report                       RepairReport
	pending                      stagedFile
	control, target, pendingBody []byte
}

func repairRefused() error {
	return failure("repair_refused", "publication evidence is inconsistent or ambiguous; retain all files for inspection")
}

func (s Store) InspectRepair(ctx context.Context) (RepairReport, error) {
	tx, err := s.openLocked(ctx)
	if err != nil {
		return RepairReport{}, err
	}
	defer tx.close()
	proof, err := tx.proveRepair(ctx, s.now())
	return proof.report, err
}
func (s Store) Repair(ctx context.Context, expect RepairExpect) (RepairReport, error) {
	if !digest.MatchString(expect.IntentSHA256) || !digest.MatchString(expect.ControlSHA256) || !digest.MatchString(expect.HistorySHA256) || expect.Sequence < 1 || expect.Sequence > maxHistoryEvents {
		return RepairReport{}, failure("invalid_arguments", "repair requires inspection hashes and sequence")
	}
	tx, err := s.openLocked(ctx)
	if err != nil {
		return RepairReport{}, err
	}
	defer tx.close()
	proof, err := tx.proveRepair(ctx, s.now())
	if err != nil {
		return RepairReport{}, err
	}
	return tx.repair(ctx, proof, expect)
}
func (t *transaction) repair(ctx context.Context, p repairProof, expect RepairExpect) (RepairReport, error) {
	r := p.report
	if expect.IntentSHA256 != r.IntentSHA256 || expect.ControlSHA256 != r.ControlSHA256 || expect.HistorySHA256 != r.JournalSHA256 || expect.Sequence != r.JournalSequence {
		return RepairReport{}, failure("repair_changed", "repair evidence changed; inspect again")
	}
	if r.ClockRollback {
		return RepairReport{}, failure("clock_rollback", "clock precedes persistent watermark; repair refused")
	}
	if err := cancelled(ctx); err != nil {
		return RepairReport{}, err
	}
	name := "repair-control-" + r.ControlSHA256 + ".bin"
	if _, err := t.root.Lstat(name); os.IsNotExist(err) {
		if _, err = t.stageEvidence(name, p.control); err != nil {
			return RepairReport{}, err
		}
	} else {
		f, err := rootedFile(t.root, name)
		if err != nil {
			return RepairReport{}, repairRefused()
		}
		err = syncMatchingEvidence(f, p.control)
		stable := unchanged(t.root, name, f)
		closeErr := f.Close()
		if err != nil {
			return RepairReport{}, err
		}
		if !stable || closeErr != nil {
			return RepairReport{}, stateIO()
		}
	}
	// A complete marker may have been left by a crash between Write and Sync.
	// Re-establish its durability before risking another control-file rewrite.
	marker, err := rootedFile(t.root, pendingName)
	if err != nil {
		return RepairReport{}, err
	}
	info, statErr := marker.Stat()
	if statErr != nil || !os.SameFile(info, p.pending.info) {
		marker.Close()
		return RepairReport{}, stateIO()
	}
	err = syncMatchingEvidence(marker, p.pendingBody)
	stable := unchanged(t.root, pendingName, marker)
	closeErr := marker.Close()
	if err != nil {
		return RepairReport{}, err
	}
	if !stable || closeErr != nil {
		return RepairReport{}, stateIO()
	}
	if err := syncStoreDirectory(t.root); err != nil {
		return RepairReport{}, stateIO()
	}
	if !t.identitiesUnchanged() {
		return RepairReport{}, stateIO()
	}
	if err := cancelled(ctx); err != nil {
		return RepairReport{}, err
	}
	if !bytes.Equal(p.control, p.target) {
		if err := publishManifest(t.file, p.target); err != nil {
			return RepairReport{}, err
		}
		r.ControlRewritten = true
	} else if t.file.Sync() != nil {
		return RepairReport{}, publicationUncertain()
	}
	if !removeOwned(t.root, p.pending) || syncStoreDirectory(t.root) != nil {
		return RepairReport{}, publicationUncertain()
	}
	r.Mode = "local_manifest_repair"
	r.Changed = true
	r.BeforeImage = name
	return r, nil
}

// A previous attempt may have written identical bytes but failed its sync.
// Directory sync alone cannot make that file's contents durable on retry.
func syncMatchingEvidence(f journalFile, expected []byte) error {
	b, err := io.ReadAll(io.LimitReader(f, int64(len(expected))+1))
	if err != nil {
		return stateIO()
	}
	if !bytes.Equal(b, expected) {
		return repairRefused()
	}
	if f.Sync() != nil {
		return stateIO()
	}
	return nil
}

func (t *transaction) proveRepair(ctx context.Context, now time.Time) (repairProof, error) {
	var proof repairProof
	if _, err := t.root.Lstat(pendingName); os.IsNotExist(err) {
		return proof, failure("repair_unavailable", "no publication intent is pending; prepared files alone cannot authorize repair")
	}
	pending, stage, err := readEvidence(t.root, pendingName, maxIntentBytes)
	if err != nil {
		return proof, err
	}
	p, err := parseIntent(pending)
	if err != nil {
		return proof, err
	}
	immutable, _, err := readEvidence(t.root, "publication-"+p.OperationID+".intent.json", maxIntentBytes)
	if err != nil {
		return proof, err
	}
	if !bytes.Equal(pending, immutable) {
		return proof, repairRefused()
	}
	source, _, err := readEvidence(t.root, p.SourceFile, maxJournalBytes)
	if err != nil {
		return proof, err
	}
	if sum(source) != p.SourceSHA256 {
		return proof, repairRefused()
	}
	target, _, err := readEvidence(t.root, p.PreparedManifest, maxManifestBytes)
	if err != nil {
		return proof, err
	}
	m, err := parseManifest(target)
	if err != nil {
		return proof, err
	}
	d := m.Segments[len(m.Segments)-1]
	if m.Hash != p.ManifestSHA256 || m.Active != "active-"+p.OperationID+".jsonl" || d.Name != "segment-"+p.OperationID+".jsonl.gz" || d.SHA256 != p.SourceActiveSHA256 || d.Bytes != p.SourceActiveBytes || m.Head != p.Head || m.SealedSequence != p.Sequence || m.At != p.RecordedAt {
		return proof, repairRefused()
	}
	var old journalState
	if p.SourceActiveName == "journal.jsonl" {
		old, err = replay(source)
		if err != nil {
			return proof, err
		}
		if m.Generation != 1 || m.Previous != sum(source) || sum(source) != p.SourceActiveSHA256 || int64(len(source)) != p.SourceActiveBytes {
			return proof, repairRefused()
		}
	} else {
		prior, parseErr := parseManifest(source)
		if parseErr != nil {
			return proof, parseErr
		}
		if prior.Active != p.SourceActiveName || m.StoreID != prior.StoreID || m.Generation != prior.Generation+1 || m.Previous != prior.Hash || !reflect.DeepEqual(m.Segments[:len(m.Segments)-1], prior.Segments) {
			return proof, repairRefused()
		}
		active, _, readErr := readEvidence(t.root, prior.Active, maxJournalBytes)
		if readErr != nil {
			return proof, readErr
		}
		if int64(len(active)) != p.SourceActiveBytes || sum(active) != p.SourceActiveSHA256 {
			return proof, repairRefused()
		}
		v := &transaction{root: t.root}
		err = v.loadManifest(ctx, prior)
		if v.file != nil {
			v.file.Close()
		}
		if err != nil {
			return proof, err
		}
		old = v.state
	}
	if old.sequence != p.Sequence || old.hash != p.Head || stamp(old.at) != p.RecordedAt {
		return proof, repairRefused()
	}
	if _, err = t.file.Seek(0, io.SeekStart); err != nil {
		return proof, stateIO()
	}
	control, err := io.ReadAll(io.LimitReader(t.file, maxJournalBytes+1))
	if err != nil {
		return proof, stateIO()
	}
	state := "torn"
	switch {
	case bytes.Equal(control, target):
		state = "published"
	case bytes.Equal(control, source):
		state = "source"
	case len(control) < len(target) && bytes.HasPrefix(target, control):
	default:
		return proof, repairRefused()
	}
	v := &transaction{root: t.root}
	err = v.loadManifest(ctx, m)
	if v.file != nil {
		v.file.Close()
	}
	if err != nil {
		return proof, err
	}
	if state != "published" && (v.size != 0 || !reflect.DeepEqual(old, v.state)) {
		return proof, repairRefused()
	}
	if err = t.checkRepairInventory(ctx, m); err != nil {
		return proof, err
	}
	if now.IsZero() {
		return proof, failure("invalid_arguments", "clock must be set")
	}
	if !t.identitiesUnchanged() {
		return proof, stateIO()
	}
	proof = repairProof{pending: stage, control: control, target: target, pendingBody: pending, report: RepairReport{Mode: "local_manifest_repair_inspection", IntentSHA256: p.Hash, ControlSHA256: sum(control), ManifestSHA256: m.Hash, JournalSequence: v.state.sequence, JournalSHA256: v.state.hash, RecordedAt: stamp(v.state.at), ObservedAt: stamp(now), ClockRollback: now.Before(v.state.at), ControlState: state, PreparedManifest: p.PreparedManifest, RetainedReservations: len(v.state.jobs)}}
	return proof, cancelled(ctx)
}

// Inventory only vetoes repair. It never elects a candidate or guesses authority.
// Retained superseded active files must still equal their sealed successors.
func (t *transaction) checkRepairInventory(ctx context.Context, m journalManifest) error {
	dir, err := t.root.Open(".")
	if err != nil {
		return stateIO()
	}
	defer dir.Close()
	entries, err := dir.ReadDir(257)
	if err != nil && err != io.EOF {
		return stateIO()
	}
	if len(entries) > 256 {
		return repairRefused()
	}
	allowed := map[string]bool{}
	oldActive := map[string]historySegment{}
	for i, d := range m.Segments {
		id := strings.TrimSuffix(strings.TrimPrefix(d.Name, "segment-"), ".jsonl.gz")
		a := "active-" + id + ".jsonl"
		allowed[d.Name] = true
		allowed[a] = true
		allowed["manifest-"+id+".json"] = true
		if i+1 < len(m.Segments) {
			oldActive[a] = m.Segments[i+1]
		}
	}
	for _, entry := range entries {
		if err = cancelled(ctx); err != nil {
			return err
		}
		name := entry.Name()
		if (strings.HasPrefix(name, "segment-") || strings.HasPrefix(name, "active-") || strings.HasPrefix(name, "manifest-")) && !allowed[name] {
			return repairRefused()
		}
		if d, ok := oldActive[name]; ok {
			b, _, err := readEvidence(t.root, name, maxJournalBytes)
			if err != nil {
				return err
			}
			if int64(len(b)) != d.Bytes || sum(b) != d.SHA256 {
				return repairRefused()
			}
		}
	}
	return nil
}
