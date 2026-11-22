package admission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Store is local accounting, not an execution service or authentication boundary.
// Dir, its ancestry, the policy and Clock must be controlled by the operator.
type Store struct {
	Dir   string
	Clock func() time.Time
}
type Config struct {
	SchemaVersion    int   `json:"schema_version"`
	MaxActive        int   `json:"max_active"`
	MaxPerPrincipal  int   `json:"max_per_principal"`
	MaxReservedBytes int64 `json:"max_reserved_bytes"`
	MaxLeaseSeconds  int   `json:"max_lease_seconds"`
}
type Reservation struct {
	ID             string `json:"id"`
	RequestID      string `json:"request_id"`
	Principal      string `json:"principal"`
	ProjectID      string `json:"project_id"`
	GrantID        string `json:"grant_id"`
	Operation      string `json:"operation"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	ReservedBytes  int64  `json:"reserved_bytes"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at"`
	Status         string `json:"status"`
}
type Receipt struct {
	Mode                   string        `json:"mode"`
	AuthenticationVerified bool          `json:"authentication_verified"`
	ExecutionAuthorized    bool          `json:"execution_authorized"`
	CheckedAt              string        `json:"checked_at"`
	Reservations           []Reservation `json:"reservations"`
}

func (c Config) valid() bool {
	return c.SchemaVersion == 1 && c.MaxActive >= 1 && c.MaxActive <= 64 && c.MaxPerPrincipal >= 1 && c.MaxPerPrincipal <= c.MaxActive && c.MaxReservedBytes >= 1 && c.MaxReservedBytes <= 16<<30 && c.MaxLeaseSeconds >= 1 && c.MaxLeaseSeconds <= 3600
}
func LoadConfig(path string) (Config, error) {
	b, err := readJSON(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if strictJSON(b, &c) != nil || !c.valid() {
		return Config{}, failure("invalid_config", "invalid reservation limits")
	}
	return c, nil
}
func (s Store) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func sum(b []byte) string      { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func requestHash(principal string, r Request) string {
	b, _ := json.Marshal(struct {
		Principal string
		Request   Request
	}{principal, r})
	return sum(b)
}
func receipt(at time.Time, rs ...Reservation) Receipt {
	if rs == nil {
		rs = []Reservation{}
	}
	return Receipt{Mode: "local_reservation", CheckedAt: stamp(at), Reservations: rs}
}

// Reserve durably consumes the request ID and nonce only after policy and actual
// archive verification. Exact retry is an error; use Status to recover a lost ack.
func (s Store) Reserve(ctx context.Context, principal, policyPath string, r Request, archive string) (Receipt, error) {
	if !identifier.MatchString(principal) {
		return Receipt{}, failure("invalid_arguments", "invalid principal")
	}
	if validateRequest(r) != nil || r.RequestID == "" {
		return Receipt{}, invalidRequest()
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return Receipt{}, err
	}
	hash := requestHash(principal, r)
	nh := sum([]byte(r.Nonce))
	for _, old := range tx.state.jobs {
		if old.Principal != principal {
			continue
		}
		if old.RequestID == r.RequestID {
			if old.Fingerprint != hash {
				return Receipt{}, failure("request_conflict", "request ID was consumed by different input")
			}
			return Receipt{}, failure("replay_detected", "request already consumed; query status by request ID")
		}
	}
	// Request-ID conflicts take precedence regardless of journal insertion order.
	for _, old := range tx.state.jobs {
		if old.Principal == principal && old.NonceHash == nh {
			return Receipt{}, failure("replay_detected", "nonce already consumed")
		}
	}
	if r.LeaseSeconds > tx.state.config.MaxLeaseSeconds {
		return Receipt{}, failure("resource_limit", "lease exceeds configured maximum")
	}
	p, err := LoadPolicy(policyPath)
	if err != nil {
		return Receipt{}, err
	}
	d, err := Check(ctx, tx.state.at, principal, p, r, archive)
	if err != nil {
		return Receipt{}, err
	}
	// Reload the policy after reading the archive. No cached decision authorizes use.
	if err = tx.observe(ctx, s.now()); err != nil {
		return Receipt{}, err
	}
	p, err = LoadPolicy(policyPath)
	if err != nil {
		return Receipt{}, err
	}
	now := s.now()
	if err = tx.observe(ctx, now); err != nil {
		return Receipt{}, err
	}
	g, err := checkGrant(now, principal, p, r)
	if err != nil {
		return Receipt{}, err
	}
	if err = checkLimits(d.Snapshot, g.Limits); err != nil {
		return Receipt{}, err
	}
	bytes := d.Snapshot.ArchiveBytes + d.Snapshot.TotalBytes
	if !tx.state.capacity(principal, bytes, now) {
		return Receipt{}, failure("capacity_exceeded", "worker reservation capacity exhausted")
	}
	if len(tx.state.jobs) >= maxReservations {
		return Receipt{}, failure("state_full", "replay history is full; no entries were evicted")
	}
	end := now.Add(time.Duration(r.LeaseSeconds) * time.Second)
	grantEnd, _ := time.Parse(time.RFC3339Nano, g.ExpiresAt)
	if end.After(grantEnd) {
		end = grantEnd
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return Receipt{}, failure("io_error", "cannot generate reservation ID")
	}
	job := storedReservation{Reservation: Reservation{ID: hex.EncodeToString(id[:]), RequestID: r.RequestID, Principal: principal, ProjectID: r.ProjectID, GrantID: r.GrantID, Operation: r.Operation, SnapshotSHA256: r.SnapshotSHA256, ReservedBytes: bytes, CreatedAt: stamp(now), ExpiresAt: stamp(end), Status: "reserved"}, Fingerprint: hash, NonceHash: nh}
	if err = tx.append(ctx, event{Kind: "reserve", At: stamp(now), Job: &job}); err != nil {
		return Receipt{}, err
	}
	// A durable commit wins over cancellation: never report that it rolled back.
	return receipt(now, job.Reservation), nil
}

func (s Store) Status(ctx context.Context, principal, requestID string) (Receipt, error) {
	if !identifier.MatchString(principal) || (requestID != "" && !identifier.MatchString(requestID)) {
		return Receipt{}, failure("invalid_arguments", "invalid principal or request ID")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return Receipt{}, err
	}
	result := receipt(tx.state.at)
	for _, j := range tx.state.jobs {
		if j.Principal == principal && (requestID == "" || j.RequestID == requestID) {
			result.Reservations = append(result.Reservations, j.current(tx.state.at))
		}
	}
	return result, nil
}
func (s Store) Release(ctx context.Context, principal, id string) (Receipt, error) {
	if !identifier.MatchString(principal) || !nonce.MatchString(id) {
		return Receipt{}, failure("invalid_arguments", "invalid principal or reservation ID")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return Receipt{}, err
	}
	for _, j := range tx.state.jobs {
		if j.ID != id || j.Principal != principal {
			continue
		}
		current := j.current(tx.state.at)
		if current.Status == "reserved" {
			if err = tx.append(ctx, event{Kind: "release", At: stamp(tx.state.at), ID: id}); err != nil {
				return Receipt{}, err
			}
			current.Status = "released"
		}
		return receipt(tx.state.at, current), nil
	}
	return Receipt{}, failure("reservation_not_found", "reservation not found for this principal")
}
