package admission

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/snapshot"
)

var testNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func fixture(t testing.TB) (Policy, Request, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.swift"), []byte("print(1)"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "source.tar")
	r, err := snapshot.Create(context.Background(), root, []string{"a.swift"}, archive)
	if err != nil {
		t.Fatal(err)
	}
	g := Grant{ID: "grant-1", Principal: "developer", ProjectID: "hello", Operation: "extract", SnapshotSHA256: r.SHA256, Enabled: true, NotBefore: testNow.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339), Limits: Limits{MaxFiles: 10, MaxFileBytes: 1024, MaxSourceBytes: 2048, MaxArchiveBytes: 16384}}
	return Policy{SchemaVersion: 1, Grants: []Grant{g}}, Request{SchemaVersion: 1, GrantID: g.ID, ProjectID: g.ProjectID, Operation: g.Operation, SnapshotSHA256: g.SnapshotSHA256}, archive
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("want %s got %v", want, err)
	}
}

func TestOfflineAdmissionIsBoundAndNotAuthorization(t *testing.T) {
	p, r, a := fixture(t)
	d, err := Check(context.Background(), testNow, "developer", p, r, a)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Eligible || d.Mode != "offline_policy_check" || d.AuthenticationVerified || d.ExecutionAuthorized || d.Snapshot.SHA256 != r.SnapshotSHA256 || d.Principal != "developer" || d.ProjectID != "hello" {
		t.Fatalf("false authority: %+v", d)
	}
	files, _ := os.ReadDir(filepath.Dir(a))
	if len(files) != 1 {
		t.Fatal("admission changed filesystem")
	}
}

func TestBindingsAndValidityDeniedBeforeArchiveRead(t *testing.T) {
	for _, mutate := range []func(*Policy, *Request, *string){
		func(_ *Policy, _ *Request, s *string) { *s = "attacker" },
		func(_ *Policy, r *Request, _ *string) { r.ProjectID = "other" },
		func(_ *Policy, r *Request, _ *string) { r.GrantID = "missing" },
		func(_ *Policy, r *Request, _ *string) { r.SnapshotSHA256 = strings.Repeat("0", 64) },
		func(p *Policy, _ *Request, _ *string) { p.Grants[0].Enabled = false },
		func(p *Policy, _ *Request, _ *string) { p.Grants[0].ExpiresAt = testNow.Format(time.RFC3339) },
		func(p *Policy, _ *Request, _ *string) {
			p.Grants[0].NotBefore = testNow.Add(time.Minute).Format(time.RFC3339)
		},
	} {
		p, r, _ := fixture(t)
		principal := "developer"
		mutate(&p, &r, &principal)
		d, err := Check(context.Background(), testNow, principal, p, r, "does-not-exist")
		code(t, err, "admission_denied")
		if d.Eligible || len(d.Snapshot.Files) > 0 {
			t.Fatal("denied report accepted")
		}
	}
	p, r, a := fixture(t)
	r.Operation = "build"
	_, err := Check(context.Background(), testNow, "developer", p, r, a)
	code(t, err, "invalid_request")
	for _, principal := range []string{"", "../actor", "actor\nforged"} {
		_, err = Check(context.Background(), testNow, principal, p, r, a)
		code(t, err, "invalid_arguments")
	}
}

func TestPolicyResourceCapsAndArchiveDigest(t *testing.T) {
	for _, mutate := range []func(*Limits){func(l *Limits) { l.MaxArchiveBytes = 10 }, func(l *Limits) { l.MaxFileBytes = 1 }, func(l *Limits) { l.MaxSourceBytes = 1 }} {
		p, r, a := fixture(t)
		mutate(&p.Grants[0].Limits)
		_, err := Check(context.Background(), testNow, "developer", p, r, a)
		code(t, err, "resource_limit")
	}
	p, r, a := fixture(t)
	if err := os.WriteFile(a, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Check(context.Background(), testNow, "developer", p, r, a)
	code(t, err, "invalid_archive")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Check(ctx, testNow, "developer", p, r, a)
	code(t, err, "cancelled")
}

func TestPolicyAndRequestParsingIsStrictAndBounded(t *testing.T) {
	p, r, _ := fixture(t)
	pb, _ := json.Marshal(p)
	rb, _ := json.Marshal(r)
	if _, err := ParsePolicy(pb); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(rb); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, []byte(`null`), append(append([]byte{}, pb...), []byte(`{}`)...), []byte(strings.Replace(string(pb), `"schema_version":1`, `"schema_version":2,"schema_version":1`, 1)), []byte(strings.Replace(string(pb), `"schema_version":1`, `"schema_version":1,"Schema_Version":1`, 1)), []byte(strings.Replace(string(pb), `"schema_version":1`, `"schema_version":1,"\u017fchema_version":1`, 1)), []byte(strings.Replace(string(pb), `"schema_version":1`, `"schema_version":1,"shell":"evil"`, 1)), []byte(strings.Repeat(" ", MaxJSONBytes+1))} {
		if _, err := ParsePolicy(bad); err == nil {
			t.Fatalf("bad policy accepted: %.200s", bad)
		}
	}
	for _, bad := range [][]byte{[]byte(`{}`), []byte(`null`), []byte(strings.Replace(string(rb), `"operation":"extract"`, `"operation":"extract","principal":"attacker"`, 1)), []byte(strings.Replace(string(rb), `"schema_version":1`, `"schema_version":1.0`, 1)), []byte(strings.Replace(string(rb), `"operation":"extract"`, `"operation":null`, 1))} {
		if _, err := ParseRequest(bad); err == nil {
			t.Fatalf("bad request accepted %s", bad)
		}
	}
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.Grants = append(p.Grants, p.Grants[0]) }, func(p *Policy) { p.Grants[0].ID = "../grant" }, func(p *Policy) { p.Grants[0].Limits.MaxFiles = 0 }, func(p *Policy) { p.Grants[0].Limits.MaxArchiveBytes = snapshot.MaxArchiveBytes + 1 }, func(p *Policy) { p.Grants[0].ExpiresAt = testNow.Add(25 * time.Hour).Format(time.RFC3339) }, func(p *Policy) { p.Grants[0].NotBefore = "bad" }, func(p *Policy) { p.Grants[0].Operation = "sign" }, func(p *Policy) { p.SchemaVersion = 2 },
	} {
		p, _, _ := fixture(t)
		mutate(&p)
		body, _ := json.Marshal(p)
		if _, err := ParsePolicy(body); err == nil {
			t.Fatalf("invalid policy accepted %s", body)
		}
	}
}

func TestLoadFilesNoFollowingSymlinks(t *testing.T) {
	p, r, _ := fixture(t)
	dir := t.TempDir()
	pp := filepath.Join(dir, "policy.json")
	rp := filepath.Join(dir, "request.json")
	pb, _ := json.Marshal(p)
	rb, _ := json.Marshal(r)
	if err := os.WriteFile(pp, pb, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rp, rb, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(pp); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRequest(rp); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(dir); err == nil {
		t.Fatal("directory accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(pp, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := LoadPolicy(link); err == nil {
		t.Fatal("policy symlink accepted")
	}
}

func FuzzAdmissionJSON(f *testing.F) {
	p, r, _ := fixture(f)
	pb, _ := json.Marshal(p)
	rb, _ := json.Marshal(r)
	f.Add(pb, true)
	f.Add(rb, false)
	f.Add([]byte(`null`), true)
	f.Fuzz(func(t *testing.T, b []byte, policy bool) {
		if policy {
			p, err := ParsePolicy(b)
			if err == nil && (p.SchemaVersion != 1 || len(p.Grants) == 0 || len(p.Grants) > MaxGrants) {
				t.Fatal("invalid policy admitted")
			}
		} else {
			r, err := ParseRequest(b)
			if err == nil && (r.SchemaVersion != 1 || r.Operation != "extract") {
				t.Fatal("invalid request admitted")
			}
		}
	})
}
