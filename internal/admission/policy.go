// Package admission evaluates offline worker policy. It does not authenticate or authorize execution.
package admission

import (
	"regexp"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

const MaxJSONBytes = 64 << 10
const MaxGrants = 64
const MaxGrantLifetime = 24 * time.Hour

type Limits struct {
	MaxFiles        int   `json:"max_files"`
	MaxFileBytes    int64 `json:"max_file_bytes"`
	MaxSourceBytes  int64 `json:"max_source_bytes"`
	MaxArchiveBytes int64 `json:"max_archive_bytes"`
}
type Grant struct {
	ID             string `json:"id"`
	Principal      string `json:"principal"`
	ProjectID      string `json:"project_id"`
	Operation      string `json:"operation"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	Enabled        bool   `json:"enabled"`
	NotBefore      string `json:"not_before"`
	ExpiresAt      string `json:"expires_at"`
	Limits         Limits `json:"limits"`
}
type Policy struct {
	SchemaVersion int     `json:"schema_version"`
	Grants        []Grant `json:"grants"`
}
type Request struct {
	SchemaVersion  int    `json:"schema_version"`
	GrantID        string `json:"grant_id"`
	ProjectID      string `json:"project_id"`
	Operation      string `json:"operation"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	RequestID      string `json:"request_id,omitempty"`
	Nonce          string `json:"nonce,omitempty"`
	LeaseSeconds   int    `json:"lease_seconds,omitempty"`
}
type Decision struct {
	Mode                   string          `json:"mode"`
	Eligible               bool            `json:"eligible"`
	AuthenticationVerified bool            `json:"authentication_verified"`
	ExecutionAuthorized    bool            `json:"execution_authorized"`
	Principal              string          `json:"principal"`
	ProjectID              string          `json:"project_id"`
	GrantID                string          `json:"grant_id"`
	Operation              string          `json:"operation"`
	CheckedAt              string          `json:"checked_at"`
	ValidUntil             string          `json:"valid_until"`
	Snapshot               snapshot.Report `json:"snapshot"`
}
type Error struct{ Code, Message string }

func (e *Error) Error() string           { return e.Message }
func failure(code, message string) error { return &Error{code, message} }

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var nonce = regexp.MustCompile(`^[a-f0-9]{32}$`)

func invalidPolicy() error {
	return failure("invalid_policy", "policy is malformed, ambiguous or outside the supported schema and limits")
}
func invalidRequest() error {
	return failure("invalid_request", "request must identify one grant, project, extract operation and snapshot SHA-256")
}

func validateRequest(r Request) error {
	if r.SchemaVersion != 1 || !identifier.MatchString(r.GrantID) || !identifier.MatchString(r.ProjectID) || r.Operation != "extract" || !digest.MatchString(r.SnapshotSHA256) {
		return invalidRequest()
	}
	if r.RequestID != "" || r.Nonce != "" || r.LeaseSeconds != 0 {
		if !identifier.MatchString(r.RequestID) || !nonce.MatchString(r.Nonce) || r.LeaseSeconds < 1 || r.LeaseSeconds > 3600 {
			return invalidRequest()
		}
	}
	return nil
}
func validatePolicy(p Policy) error {
	if p.SchemaVersion != 1 || len(p.Grants) == 0 || len(p.Grants) > MaxGrants {
		return invalidPolicy()
	}
	seen := map[string]bool{}
	for _, g := range p.Grants {
		if !identifier.MatchString(g.ID) || seen[g.ID] || !identifier.MatchString(g.Principal) || !identifier.MatchString(g.ProjectID) || g.Operation != "extract" || !digest.MatchString(g.SnapshotSHA256) {
			return invalidPolicy()
		}
		seen[g.ID] = true
		start, e1 := time.Parse(time.RFC3339Nano, g.NotBefore)
		end, e2 := time.Parse(time.RFC3339Nano, g.ExpiresAt)
		if e1 != nil || e2 != nil || !start.Before(end) || end.Sub(start) > MaxGrantLifetime {
			return invalidPolicy()
		}
		l := g.Limits
		if l.MaxFiles <= 0 || l.MaxFiles > snapshot.MaxFiles || l.MaxFileBytes <= 0 || l.MaxFileBytes > snapshot.MaxFileBytes || l.MaxSourceBytes <= 0 || l.MaxSourceBytes > snapshot.MaxTotalBytes || l.MaxArchiveBytes <= 0 || l.MaxArchiveBytes > snapshot.MaxArchiveBytes {
			return invalidPolicy()
		}
	}
	return nil
}
