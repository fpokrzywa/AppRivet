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
		if j.current(now).Status == "reserved" {
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
	if err != nil || now.IsZero() || stamp(now) != e.At || now.Before(s.at) || e.Sequence != s.sequence+1 || e.Previous != s.hash {
		return corrupt()
	}
	hash := e.Hash
	e.Hash = ""
	b, _ := json.Marshal(e)
	if !digest.MatchString(hash) || sum(b) != hash {
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
	if len(b) == 0 || len(b) > maxJournalBytes || b[len(b)-1] != '\n' {
		return s, corrupt()
	}
	lines := bytes.Split(b[:len(b)-1], []byte{'\n'})
	if len(lines) > maxEvents {
		return s, corrupt()
	}
	for _, line := range lines {
		if len(line) > maxEventBytes {
			return journalState{}, corrupt()
		}
		var e event
		if json.Unmarshal(line, &e) != nil {
			return journalState{}, corrupt()
		}
		canonical, _ := json.Marshal(e)
		if !bytes.Equal(line, canonical) {
			return journalState{}, corrupt()
		}
		if err := s.apply(e); err != nil {
			return journalState{}, err
		}
	}
	return s, nil
}

type journalFile interface {
	io.ReadWriteSeeker
	Stat() (os.FileInfo, error)
	Sync() error
	Close() error
}

type transaction struct {
	root  *os.Root
	lock  *os.File
	file  journalFile
	state journalState
	size  int64
}

func (t *transaction) close() {
	if t.file != nil {
		t.file.Close()
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
func (s Store) begin(ctx context.Context) (*transaction, error) {
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
	tx := &transaction{root: root}
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
	b, err := io.ReadAll(io.LimitReader(tx.file, maxJournalBytes+1))
	if err != nil {
		return nil, stateIO()
	}
	tx.state, err = replay(b)
	if err != nil {
		return nil, err
	}
	tx.size = int64(len(b))
	ok = true
	return tx, nil
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
	if t.state.sequence >= maxEvents || len(b) > maxEventBytes+1 || t.size+int64(len(b)) > maxJournalBytes {
		return failure("state_full", "reservation journal is full; replay history was retained")
	}
	// Validate the next transition before writing, without mutating live state.
	next := t.state
	next.jobs = append([]storedReservation(nil), t.state.jobs...)
	if err := next.apply(e); err != nil {
		return err
	}
	if !unchanged(t.root, "lock", t.lock) || !unchanged(t.root, "journal.jsonl", t.file) {
		return stateIO()
	}
	if err := cancelled(ctx); err != nil {
		return err
	}
	if _, err := t.file.Seek(0, io.SeekEnd); err != nil {
		return stateIO()
	}
	n, err := t.file.Write(b)
	if err != nil || n != len(b) {
		return failure("commit_uncertain", "journal write failed; retain state and query status before retrying")
	}
	if err = t.file.Sync(); err != nil {
		return failure("commit_uncertain", "journal sync failed; retain state and query status before retrying")
	}
	t.state = next
	t.size += int64(len(b))
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
	tx := &transaction{root: root}
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
