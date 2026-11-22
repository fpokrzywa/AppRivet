package admission

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"time"
)

const maxHistoryBytes int64 = 64 << 20
const maxHistoryEvents = 65536
const maxSegments = 16
const maxManifestBytes = 64 << 10
const maxCompressedBytes = maxJournalBytes + (64 << 10)

type historySegment struct {
	Name             string `json:"name"`
	Bytes            int64  `json:"bytes"`
	Events           int    `json:"events"`
	SHA256           string `json:"sha256"`
	CompressedBytes  int64  `json:"compressed_bytes"`
	CompressedSHA256 string `json:"compressed_sha256"`
}
type journalManifest struct {
	SchemaVersion  int              `json:"schema_version"`
	StoreID        string           `json:"store_id"`
	Generation     int              `json:"generation"`
	Previous       string           `json:"previous_manifest_sha256"`
	SealedSequence int              `json:"sealed_sequence"`
	Head           string           `json:"head_sha256"`
	At             string           `json:"recorded_at"`
	Segments       []historySegment `json:"segments"`
	Active         string           `json:"active"`
	Hash           string           `json:"hash"`
}

var segmentName = regexp.MustCompile(`^segment-[a-f0-9]{32}\.jsonl\.gz$`)
var activeName = regexp.MustCompile(`^active-[a-f0-9]{32}\.jsonl$`)

func manifestBytes(m journalManifest) []byte {
	m.Hash = ""
	b, _ := json.Marshal(m)
	m.Hash = sum(b)
	b, _ = json.Marshal(m)
	return append(b, '\n')
}
func parseManifest(b []byte) (journalManifest, error) {
	var m journalManifest
	if len(b) > maxManifestBytes || json.Unmarshal(b, &m) != nil {
		return m, corrupt()
	}
	if !bytes.Equal(b, manifestBytes(m)) || !digest.MatchString(m.Hash) {
		return journalManifest{}, corrupt()
	}
	at, err := time.Parse(time.RFC3339Nano, m.At)
	if m.SchemaVersion != 2 || !nonce.MatchString(m.StoreID) || m.Generation < 1 || m.Generation > maxSegments || m.Generation != len(m.Segments) || !digest.MatchString(m.Previous) || !digest.MatchString(m.Head) || m.SealedSequence < 1 || m.SealedSequence > maxHistoryEvents || err != nil || at.IsZero() || stamp(at) != m.At || !activeName.MatchString(m.Active) {
		return journalManifest{}, corrupt()
	}
	names := map[string]bool{}
	var totalBytes int64
	events := 0
	for _, d := range m.Segments {
		if !segmentName.MatchString(d.Name) || names[d.Name] || d.Bytes < 1 || d.Bytes > maxJournalBytes || d.Events < 1 || d.Events > maxEvents || !digest.MatchString(d.SHA256) || d.CompressedBytes < 1 || d.CompressedBytes > maxCompressedBytes || !digest.MatchString(d.CompressedSHA256) {
			return journalManifest{}, corrupt()
		}
		names[d.Name] = true
		totalBytes += d.Bytes
		events += d.Events
	}
	if totalBytes > maxHistoryBytes || events != m.SealedSequence {
		return journalManifest{}, corrupt()
	}
	return m, nil
}
func unpackSegment(packed []byte, d historySegment) ([]byte, error) {
	if d.Bytes < 1 || d.Bytes > maxJournalBytes || d.CompressedBytes < 1 || d.CompressedBytes > maxCompressedBytes || int64(len(packed)) != d.CompressedBytes || sum(packed) != d.CompressedSHA256 {
		return nil, corrupt()
	}
	input := bytes.NewReader(packed)
	z, err := gzip.NewReader(input)
	if err != nil {
		return nil, corrupt()
	}
	defer z.Close()
	z.Multistream(false)
	b, err := io.ReadAll(io.LimitReader(z, d.Bytes+1))
	if err != nil || int64(len(b)) != d.Bytes || input.Len() != 0 || sum(b) != d.SHA256 {
		return nil, corrupt()
	}
	return b, nil
}
func (t *transaction) loadManifest(ctx context.Context, m journalManifest) error {
	if err := t.loadSealedHistory(ctx, m); err != nil {
		return err
	}
	b, err := t.openActive(m)
	if err != nil {
		return err
	}
	if err = t.replayActive(b, true); err != nil {
		return err
	}
	return cancelled(ctx)
}

func (t *transaction) loadSealedHistory(ctx context.Context, m journalManifest) error {
	for _, d := range m.Segments {
		if err := cancelled(ctx); err != nil {
			return err
		}
		f, err := rootedFile(t.root, d.Name)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil || info.Size() != d.CompressedBytes {
			f.Close()
			return corrupt()
		}
		packed, err := io.ReadAll(io.LimitReader(f, d.CompressedBytes+1))
		f.Close()
		if err != nil {
			return stateIO()
		}
		b, err := unpackSegment(packed, d)
		if err != nil {
			return err
		}
		n, err := replaySegment(&t.state, b, false)
		if err != nil || n != d.Events {
			return corrupt()
		}
		t.historyBytes += int64(len(b))
	}
	if t.state.sequence != m.SealedSequence || t.state.hash != m.Head || stamp(t.state.at) != m.At {
		return corrupt()
	}
	return nil
}

func (t *transaction) openActive(m journalManifest) ([]byte, error) {
	f, err := rootedFile(t.root, m.Active)
	if err != nil {
		return nil, err
	}
	t.file = f
	limit := int64(maxJournalBytes)
	if remaining := maxHistoryBytes - t.historyBytes; remaining < limit {
		limit = remaining
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, stateIO()
	}
	if int64(len(b)) > limit {
		return nil, corrupt()
	}
	return b, nil
}

func (t *transaction) replayActive(b []byte, allowEmpty bool) error {
	n, err := replaySegment(&t.state, b, allowEmpty)
	if err != nil {
		return err
	}
	t.activeEvents = n
	t.size = int64(len(b))
	t.historyBytes += t.size
	return nil
}
func (t *transaction) identitiesUnchanged() bool {
	return unchanged(t.root, "lock", t.lock) && unchanged(t.root, t.fileName, t.file) && (t.control == nil || unchanged(t.root, "journal.jsonl", t.control))
}
