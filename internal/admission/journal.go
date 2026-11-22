package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

const maxJournalBytes = 16 << 20
const maxEvents = 16384
const maxEventBytes = 4096
const maxReservations = 4096

type storedReservation struct {
	Reservation
	Fingerprint string `json:"fingerprint"`
	NonceHash   string `json:"nonce_hash"`
}

func (j storedReservation) current(now time.Time) Reservation {
	r := j.Reservation
	end, _ := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if r.Status == "reserved" && !now.Before(end) {
		r.Status = "expired"
	}
	if r.Status == "claimed" && !now.Before(end) {
		r.Status = "recovery_required"
	}
	return r
}

type event struct {
	Sequence int                `json:"sequence"`
	Previous string             `json:"previous"`
	At       string             `json:"at"`
	Kind     string             `json:"kind"`
	Config   *Config            `json:"config,omitempty"`
	Job      *storedReservation `json:"job,omitempty"`
	ID       string             `json:"id,omitempty"`
	Owner    *Ownership         `json:"owner,omitempty"`
	Stopped  bool               `json:"confirmed_stopped,omitempty"`
	Hash     string             `json:"hash"`
}
type journalState struct {
	config   Config
	jobs     []storedReservation
	at       time.Time
	sequence int
	hash     string
}

func corrupt() error {
	return failure("state_corrupt", "reservation journal is incomplete, damaged or invalid; no automatic reset is permitted")
}
func (s *journalState) capacity(principal string, n int64, now time.Time) bool {
	active, own := 0, 0
	var used int64
	for _, j := range s.jobs {
		if j.current(now).Status == "reserved" || j.Status == "claimed" {
			active++
			used += j.ReservedBytes
			if j.Principal == principal {
				own++
			}
		}
	}
	return n > 0 && active < s.config.MaxActive && own < s.config.MaxPerPrincipal && n <= s.config.MaxReservedBytes-used
}
func (s *journalState) apply(e event) error {
	now, err := time.Parse(time.RFC3339Nano, e.At)
	if err != nil || now.IsZero() || stamp(now) != e.At || now.Before(s.at) || e.Sequence != s.sequence+1 || e.Sequence > maxHistoryEvents || e.Previous != s.hash {
		return corrupt()
	}
	hash := e.Hash
	e.Hash = ""
	b, _ := json.Marshal(e)
	if !digest.MatchString(hash) || sum(b) != hash {
		return corrupt()
	}
	if e.Kind != "claim" && e.Kind != "recover" && (e.Owner != nil || e.Stopped) {
		return corrupt()
	}
	switch e.Kind {
	case "init":
		if s.sequence != 0 || e.Config == nil || !e.Config.valid() || e.Job != nil || e.ID != "" {
			return corrupt()
		}
		s.config = *e.Config
	case "observe":
		if s.sequence == 0 || e.Config != nil || e.Job != nil || e.ID != "" {
			return corrupt()
		}
	case "reserve":
		if s.sequence == 0 || e.Config != nil || e.Job == nil || e.ID != "" || len(s.jobs) >= maxReservations {
			return corrupt()
		}
		j := *e.Job
		if j.WorkerID != "" || j.ClaimID != "" || j.Fence != 0 || j.ClaimedAt != "" {
			return corrupt()
		}
		end, err := time.Parse(time.RFC3339Nano, j.ExpiresAt)
		if !nonce.MatchString(j.ID) || !identifier.MatchString(j.RequestID) || !identifier.MatchString(j.Principal) || !identifier.MatchString(j.ProjectID) || !identifier.MatchString(j.GrantID) || j.Operation != "extract" || !digest.MatchString(j.SnapshotSHA256) || !digest.MatchString(j.Fingerprint) || !digest.MatchString(j.NonceHash) || j.Status != "reserved" || j.CreatedAt != e.At || err != nil || stamp(end) != j.ExpiresAt || !end.After(now) || end.Sub(now) > time.Duration(s.config.MaxLeaseSeconds)*time.Second || j.ReservedBytes > snapshot.MaxArchiveBytes+snapshot.MaxTotalBytes || !s.capacity(j.Principal, j.ReservedBytes, now) {
			return corrupt()
		}
		for _, old := range s.jobs {
			if old.ID == j.ID || (old.Principal == j.Principal && (old.RequestID == j.RequestID || old.NonceHash == j.NonceHash)) {
				return corrupt()
			}
		}
		s.jobs = append(s.jobs, j)
	case "release":
		if s.sequence == 0 || e.Config != nil || e.Job != nil || !nonce.MatchString(e.ID) {
			return corrupt()
		}
		found := false
		for i := range s.jobs {
			if s.jobs[i].ID == e.ID {
				if s.jobs[i].current(now).Status != "reserved" {
					return corrupt()
				}
				s.jobs[i].Status = "released"
				found = true
				break
			}
		}
		if !found {
			return corrupt()
		}
	case "claim", "recover":
		if err := s.applyOwnership(e, now); err != nil {
			return err
		}
	default:
		return corrupt()
	}
	s.at = now
	s.sequence = e.Sequence
	s.hash = hash
	return nil
}
func replay(b []byte) (journalState, error) {
	var s journalState
	_, err := replaySegment(&s, b, false)
	if err != nil {
		return journalState{}, err
	}
	return s, nil
}
func replaySegment(s *journalState, b []byte, allowEmpty bool) (int, error) {
	if len(b) == 0 && allowEmpty {
		return 0, nil
	}
	if len(b) == 0 || len(b) > maxJournalBytes || b[len(b)-1] != '\n' {
		return 0, corrupt()
	}
	lines := bytes.Split(b[:len(b)-1], []byte{'\n'})
	if len(lines) > maxEvents {
		return 0, corrupt()
	}
	for _, line := range lines {
		if len(line) > maxEventBytes {
			return 0, corrupt()
		}
		var e event
		if json.Unmarshal(line, &e) != nil {
			return 0, corrupt()
		}
		canonical, _ := json.Marshal(e)
		if !bytes.Equal(line, canonical) {
			return 0, corrupt()
		}
		if err := s.apply(e); err != nil {
			return 0, err
		}
	}
	return len(lines), nil
}

type journalFile interface {
	io.ReadWriteSeeker
	Stat() (os.FileInfo, error)
	Sync() error
	Close() error
	Truncate(int64) error
}

type transaction struct {
	root         *os.Root
	lock         *os.File
	file         journalFile
	state        journalState
	size         int64
	control      journalFile
	fileName     string
	manifest     *journalManifest
	activeEvents int
	historyBytes int64
	openStage    func(string) (journalFile, error)
}

func (t *transaction) close() {
	if t.file != nil {
		t.file.Close()
	}
	if t.control != nil {
		t.control.Close()
	}
	if t.lock != nil {
		unlockFile(t.lock)
		t.lock.Close()
	}
	if t.root != nil {
		t.root.Close()
	}
}
func stateIO() error {
	return failure("state_io", "state must be an initialized, accessible directory with stable regular lock and journal files")
}
func rootedFile(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, stateIO()
	}
	f, err := root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, stateIO()
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		f.Close()
		return nil, stateIO()
	}
	return f, nil
}
func unchanged(root *os.Root, name string, f interface{ Stat() (os.FileInfo, error) }) bool {
	named, err := root.Lstat(name)
	if err != nil || !named.Mode().IsRegular() {
		return false
	}
	opened, err := f.Stat()
	return err == nil && os.SameFile(named, opened)
}
func (s Store) openLocked(ctx context.Context) (*transaction, error) {
	if err := cancelled(ctx); err != nil {
		return nil, err
	}
	info, err := os.Lstat(s.Dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, stateIO()
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, stateIO()
	}
	tx := &transaction{root: root, fileName: "journal.jsonl"}
	ok := false
	defer func() {
		if !ok {
			tx.close()
		}
	}()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return nil, stateIO()
	}
	tx.lock, err = rootedFile(root, "lock")
	if err != nil {
		return nil, err
	}
	if err = lockFile(ctx, tx.lock); err != nil {
		return nil, err
	}
	if !unchanged(root, "lock", tx.lock) {
		return nil, stateIO()
	}
	tx.file, err = rootedFile(root, "journal.jsonl")
	if err != nil {
		return nil, err
	}
	ok = true
	return tx, nil
}
func (s Store) begin(ctx context.Context) (*transaction, error) {
	tx, err := s.openLocked(ctx)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			tx.close()
		}
	}()
	if err = requireNoPending(tx.root, pendingName, "publication_pending", "manifest publication requires repair inspection"); err != nil {
		return nil, err
	}
	if err = requireNoPending(tx.root, appendPendingName, "append_pending", "journal append requires append repair inspection"); err != nil {
		return nil, err
	}
	b, err := tx.selectJournal(ctx)
	if err != nil {
		return nil, err
	}
	if err = tx.replayActive(b, tx.manifest != nil); err != nil {
		return nil, err
	}
	if err = cancelled(ctx); err != nil {
		return nil, err
	}
	ok = true
	return tx, nil
}

func requireNoPending(root *os.Root, name, code, message string) error {
	if _, err := root.Lstat(name); err == nil {
		return failure(code, message)
	} else if !os.IsNotExist(err) {
		return stateIO()
	}
	return nil
}

// Select and read the active file. Replay of its bytes is deferred so repair
// can validate the exact recorded prefix before interpreting a torn tail.
func (t *transaction) selectJournal(ctx context.Context) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(t.file, maxJournalBytes+1))
	if err != nil {
		return nil, stateIO()
	}
	if bytes.HasPrefix(b, []byte(`{"schema_version":`)) {
		m, parseErr := parseManifest(b)
		if parseErr != nil {
			return nil, parseErr
		}
		t.control = t.file
		t.file = nil
		t.manifest = &m
		t.fileName = m.Active
		if err = t.loadSealedHistory(ctx, m); err != nil {
			return nil, err
		}
		return t.openActive(m)
	}
	if len(b) > maxJournalBytes {
		return nil, corrupt()
	}
	return b, nil
}
func (t *transaction) observe(ctx context.Context, now time.Time) error {
	if err := cancelled(ctx); err != nil {
		return err
	}
	if now.IsZero() || now.Before(t.state.at) {
		return failure("clock_rollback", "clock precedes persistent watermark; reservations are frozen")
	}
	if now.Equal(t.state.at) {
		return nil
	}
	return t.append(ctx, event{Kind: "observe", At: stamp(now)})
}
func (t *transaction) append(ctx context.Context, e event) error {
	if err := cancelled(ctx); err != nil {
		return err
	}
	e.Sequence = t.state.sequence + 1
	e.Previous = t.state.hash
	b, _ := json.Marshal(e)
	e.Hash = sum(b)
	b, _ = json.Marshal(e)
	b = append(b, '\n')
	if t.activeEvents >= maxEvents || t.state.sequence >= maxHistoryEvents || len(b) > maxEventBytes+1 || t.size+int64(len(b)) > maxJournalBytes || t.historyBytes+int64(len(b)) > maxHistoryBytes {
		return failure("state_full", "reservation journal is full; replay history was retained")
	}
	// Validate the next transition before writing, without mutating live state.
	next := t.state
	next.jobs = append([]storedReservation(nil), t.state.jobs...)
	if err := next.apply(e); err != nil {
		return err
	}
	if !t.identitiesUnchanged() {
		return stateIO()
	}
	if err := cancelled(ctx); err != nil {
		return err
	}
	if size, err := t.file.Seek(0, io.SeekEnd); err != nil || size != t.size {
		return stateIO()
	}
	intent := appendIntent{SchemaVersion: 1, Active: t.fileName, ManifestSHA256: t.activeManifestHash(), Offset: t.size, Record: b}
	marker, err := t.stageEvidence(appendPendingName, appendIntentBytes(intent))
	if err != nil {
		return err
	}
	if syncStoreDirectory(t.root) != nil {
		return appendUncertain()
	}
	if err = t.finishAppend(marker, b); err != nil {
		return err
	}
	t.state = next
	t.size += int64(len(b))
	t.activeEvents++
	t.historyBytes += int64(len(b))
	return nil
}

// Init exclusively creates a new store. Failed initialization is left for
// inspection, never automatically deleted or silently treated as empty state.
func (s Store) Init(ctx context.Context, c Config) error {
	if err := cancelled(ctx); err != nil {
		return err
	}
	if !c.valid() {
		return failure("invalid_config", "invalid reservation limits")
	}
	now := s.now()
	if now.IsZero() {
		return failure("invalid_arguments", "clock must be set")
	}
	path, err := filepath.Abs(s.Dir)
	if err != nil {
		return stateIO()
	}
	parent, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return stateIO()
	}
	defer parent.Close()
	if err = parent.Mkdir(filepath.Base(path), 0700); err != nil {
		return failure("state_exists", "state destination must not exist")
	}
	root, err := parent.OpenRoot(filepath.Base(path))
	if err != nil {
		return stateIO()
	}
	tx := &transaction{root: root, fileName: "journal.jsonl"}
	defer tx.close()
	tx.lock, err = root.OpenFile("lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return stateIO()
	}
	if err = lockFile(ctx, tx.lock); err != nil {
		return err
	}
	tx.file, err = root.OpenFile("journal.jsonl", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return stateIO()
	}
	return tx.append(ctx, event{Kind: "init", At: stamp(now), Config: &c})
}
