package admission

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

func cancelled(ctx context.Context) error {
	if ctx.Err() != nil {
		return failure("cancelled", "admission check cancelled; no decision accepted")
	}
	return nil
}

// Check evaluates trusted local policy at a caller-supplied trusted clock instant.
// Principal must come from an authenticated transport in a future service. This
// offline evaluator cannot establish that provenance and issues no admission token.
func Check(ctx context.Context, now time.Time, principal string, p Policy, r Request, archive string) (Decision, error) {
	if err := cancelled(ctx); err != nil {
		return Decision{}, err
	}
	if !identifier.MatchString(principal) || now.IsZero() {
		return Decision{}, failure("invalid_arguments", "principal must be a portable identifier and the clock must be set")
	}
	if err := validatePolicy(p); err != nil {
		return Decision{}, err
	}
	if err := validateRequest(r); err != nil {
		return Decision{}, err
	}
	var selected *Grant
	for i := range p.Grants {
		if p.Grants[i].ID == r.GrantID {
			selected = &p.Grants[i]
			break
		}
	}
	denied := func() (Decision, error) {
		return Decision{}, failure("admission_denied", "no active matching grant permits this principal, project, operation and snapshot")
	}
	if selected == nil {
		return denied()
	}
	g := *selected
	start, _ := time.Parse(time.RFC3339Nano, g.NotBefore)
	end, _ := time.Parse(time.RFC3339Nano, g.ExpiresAt)
	if !g.Enabled || g.Principal != principal || g.ProjectID != r.ProjectID || g.Operation != r.Operation || g.SnapshotSHA256 != r.SnapshotSHA256 || now.Before(start) || !now.Before(end) {
		return denied()
	}
	report, err := inspectArchive(ctx, archive, r.SnapshotSHA256, g.Limits)
	if cancelErr := cancelled(ctx); cancelErr != nil {
		return Decision{}, cancelErr
	}
	if err != nil {
		return Decision{}, err
	}
	return Decision{Mode: "offline_policy_check", Eligible: true, Principal: principal, ProjectID: r.ProjectID, GrantID: r.GrantID, Operation: r.Operation, CheckedAt: now.UTC().Format(time.RFC3339Nano), ValidUntil: end.UTC().Format(time.RFC3339Nano), Snapshot: report}, nil
}

type counter struct {
	r io.Reader
	n int64
}

func (c *counter) Read(p []byte) (int, error) { n, err := c.r.Read(p); c.n += int64(n); return n, err }
func resourceLimit() error {
	return failure("resource_limit", "snapshot exceeds the selected grant's resource limits")
}
func inspectArchive(ctx context.Context, path, expected string, l Limits) (snapshot.Report, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return snapshot.Report{}, failure("io_error", "archive must be an accessible regular file, not a symlink")
	}
	if info.Size() > l.MaxArchiveBytes {
		return snapshot.Report{}, resourceLimit()
	}
	f, err := os.Open(path)
	if err != nil {
		return snapshot.Report{}, failure("io_error", "cannot open archive")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return snapshot.Report{}, failure("io_error", "archive identity changed")
	}
	input := &counter{r: io.LimitReader(f, l.MaxArchiveBytes+1)}
	r, err := snapshot.Verify(ctx, input, expected)
	if input.n > l.MaxArchiveBytes {
		return snapshot.Report{}, resourceLimit()
	}
	if err != nil {
		return snapshot.Report{}, failure("invalid_archive", "archive failed canonical integrity or expected-digest verification")
	}
	if len(r.Files) > l.MaxFiles || r.TotalBytes > l.MaxSourceBytes {
		return snapshot.Report{}, resourceLimit()
	}
	for _, e := range r.Files {
		if e.Size > l.MaxFileBytes {
			return snapshot.Report{}, resourceLimit()
		}
	}
	return r, nil
}
