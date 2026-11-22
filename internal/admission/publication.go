package admission

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"time"
)

const pendingName = "publication.pending"
const maxIntentBytes = 4096
const evidenceStagePrefix = ".journal-evidence-"

// Publish evidence under its final name only after its contents are synced.
// Link never replaces a pre-existing name, including an older damaged marker.
func (t *transaction) stageEvidence(name string, body []byte) (stagedFile, error) {
	staged, err := t.stageFile(evidenceStagePrefix+rand.Text(), body)
	if staged.info != nil {
		defer removeOwned(t.root, staged)
	}
	if err != nil {
		return stagedFile{}, err
	}
	if err := t.root.Link(staged.name, name); err != nil {
		return stagedFile{}, stateIO()
	}
	return stagedFile{name: name, info: staged.info}, nil
}

func (t *transaction) syncEvidence(stage stagedFile, body []byte) error {
	f, err := rootedFile(t.root, stage.name)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil || !os.SameFile(info, stage.info) {
		f.Close()
		return stateIO()
	}
	err = syncMatchingEvidence(f, body)
	stable := unchanged(t.root, stage.name, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if !stable || closeErr != nil {
		return stateIO()
	}
	return nil
}

// An intent binds the exact pre-publication authority to one prepared successor.
// Hashes detect corruption; they do not authenticate a hostile storage owner.
type publicationIntent struct {
	SchemaVersion      int    `json:"schema_version"`
	OperationID        string `json:"operation_id"`
	PreparedManifest   string `json:"prepared_manifest"`
	ManifestSHA256     string `json:"manifest_sha256"`
	SourceFile         string `json:"source_file"`
	SourceSHA256       string `json:"source_sha256"`
	SourceActiveName   string `json:"source_active_name"`
	SourceActiveSHA256 string `json:"source_active_sha256"`
	SourceActiveBytes  int64  `json:"source_active_bytes"`
	Sequence           int    `json:"sequence"`
	Head               string `json:"head_sha256"`
	RecordedAt         string `json:"recorded_at"`
	Hash               string `json:"hash"`
}

func intentBytes(p publicationIntent) []byte {
	p.Hash = ""
	b, _ := json.Marshal(p)
	p.Hash = sum(b)
	b, _ = json.Marshal(p)
	return append(b, '\n')
}
func parseIntent(b []byte) (publicationIntent, error) {
	var p publicationIntent
	if len(b) > maxIntentBytes || json.Unmarshal(b, &p) != nil {
		return p, corrupt()
	}
	at, err := time.Parse(time.RFC3339Nano, p.RecordedAt)
	if !bytes.Equal(b, intentBytes(p)) || p.SchemaVersion != 1 || !nonce.MatchString(p.OperationID) || p.PreparedManifest != "manifest-"+p.OperationID+".json" || p.SourceFile != "publication-source-"+p.OperationID+".bin" || !digest.MatchString(p.Hash) || !digest.MatchString(p.ManifestSHA256) || !digest.MatchString(p.SourceSHA256) || !digest.MatchString(p.SourceActiveSHA256) || !digest.MatchString(p.Head) || (p.SourceActiveName != "journal.jsonl" && !activeName.MatchString(p.SourceActiveName)) || p.SourceActiveBytes < 1 || p.SourceActiveBytes > maxJournalBytes || p.Sequence < 1 || p.Sequence > maxHistoryEvents || err != nil || at.IsZero() || stamp(at) != p.RecordedAt {
		return publicationIntent{}, corrupt()
	}
	return p, nil
}

func readEvidence(root *os.Root, name string, limit int64) ([]byte, stagedFile, error) {
	f, err := rootedFile(root, name)
	if err != nil {
		return nil, stagedFile{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, stagedFile{}, stateIO()
	}
	if info.Size() > limit {
		return nil, stagedFile{}, corrupt()
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, stagedFile{}, stateIO()
	}
	if int64(len(b)) > limit || !unchanged(root, name, f) {
		return nil, stagedFile{}, corrupt()
	}
	return b, stagedFile{name, info}, nil
}
