package admission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Ownership is a local compare-and-match identity, not a bearer capability.
// A future executor must enforce it at every protected side effect.
type Ownership struct {
	ReservationID string `json:"reservation_id"`
	WorkerID      string `json:"worker_id"`
	ClaimID       string `json:"claim_id"`
	Fence         int    `json:"fence"`
}

func (o Ownership) valid() bool {
	return nonce.MatchString(o.ReservationID) && identifier.MatchString(o.WorkerID) && nonce.MatchString(o.ClaimID) && o.Fence > 0 && o.Fence <= maxHistoryEvents
}
func (o Ownership) matches(j Reservation) bool {
	return o.ReservationID == j.ID && o.WorkerID == j.WorkerID && o.ClaimID == j.ClaimID && o.Fence == j.Fence
}

type OwnershipDecision struct {
	Mode                   string      `json:"mode"`
	OwnershipValid         bool        `json:"ownership_valid"`
	AuthenticationVerified bool        `json:"authentication_verified"`
	ExecutionAuthorized    bool        `json:"execution_authorized"`
	CheckedAt              string      `json:"checked_at"`
	ValidUntil             string      `json:"valid_until"`
	Reservation            Reservation `json:"reservation"`
}
type RecoveryReceipt struct {
	Receipt
	StopAssertionVerified bool `json:"stop_assertion_verified"`
}
type RecoveryReport struct {
	Mode                   string        `json:"mode"`
	AuthenticationVerified bool          `json:"authentication_verified"`
	ExecutionAuthorized    bool          `json:"execution_authorized"`
	JournalSequence        int           `json:"journal_sequence"`
	JournalSHA256          string        `json:"journal_sha256"`
	RecordedAt             string        `json:"recorded_at"`
	ObservedAt             string        `json:"observed_at"`
	ClockRollback          bool          `json:"clock_rollback"`
	Reservations           []Reservation `json:"reservations"`
}

func ownerConflict() error {
	return failure("ownership_conflict", "reservation ownership is absent, closed or does not match the supplied worker/claim/fence")
}
func (s *journalState) owned(principal, id string) (storedReservation, error) {
	for _, j := range s.jobs {
		if j.Principal == principal && j.ID == id {
			return j, nil
		}
	}
	return storedReservation{}, failure("reservation_not_found", "reservation not found for this principal")
}
func liveClaim(j storedReservation, o Ownership, now time.Time) error {
	if !o.matches(j.Reservation) || j.Status != "claimed" {
		return ownerConflict()
	}
	if j.current(now).Status != "claimed" {
		return failure("ownership_expired", "claim expired; capacity remains held until explicit recovery")
	}
	return nil
}

// recheckOwnershipInput verifies actual bytes and reloads policy after verification.
// The lock protects ledger state, not the policy deployment or an external executor.
func (s Store) recheckOwnershipInput(ctx context.Context, tx *transaction, j storedReservation, policyPath, archive string) (time.Time, time.Time, error) {
	r := Request{SchemaVersion: 1, GrantID: j.GrantID, ProjectID: j.ProjectID, Operation: j.Operation, SnapshotSHA256: j.SnapshotSHA256}
	p, err := LoadPolicy(policyPath)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	d, err := Check(ctx, tx.state.at, j.Principal, p, r, archive)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if err = tx.observe(ctx, s.now()); err != nil {
		return time.Time{}, time.Time{}, err
	}
	p, err = LoadPolicy(policyPath)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	now := s.now()
	if err = tx.observe(ctx, now); err != nil {
		return time.Time{}, time.Time{}, err
	}
	g, err := checkGrant(now, j.Principal, p, r)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if err = checkLimits(d.Snapshot, g.Limits); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if d.Snapshot.TotalBytes+d.Snapshot.ArchiveBytes != j.ReservedBytes {
		return time.Time{}, time.Time{}, corrupt()
	}
	end, _ := time.Parse(time.RFC3339Nano, j.ExpiresAt)
	grantEnd, _ := time.Parse(time.RFC3339Nano, g.ExpiresAt)
	if grantEnd.Before(end) {
		end = grantEnd
	}
	return now, end, nil
}

func (s Store) Claim(ctx context.Context, principal, id, workerID, policyPath, archive string) (Receipt, error) {
	if !identifier.MatchString(principal) || !nonce.MatchString(id) || !identifier.MatchString(workerID) {
		return Receipt{}, failure("invalid_arguments", "invalid principal, reservation or worker ID")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return Receipt{}, err
	}
	j, err := tx.state.owned(principal, id)
	if err != nil {
		return Receipt{}, err
	}
	if j.current(tx.state.at).Status != "reserved" {
		return Receipt{}, ownerConflict()
	}
	now, _, err := s.recheckOwnershipInput(ctx, tx, j, policyPath, archive)
	if err != nil {
		return Receipt{}, err
	}
	if j.current(now).Status != "reserved" {
		return Receipt{}, failure("ownership_expired", "reservation expired during verification")
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return Receipt{}, failure("io_error", "cannot generate claim identity")
	}
	owner := Ownership{ReservationID: id, WorkerID: workerID, ClaimID: hex.EncodeToString(random[:]), Fence: tx.state.sequence + 1}
	if err = tx.append(ctx, event{Kind: "claim", At: stamp(now), ID: id, Owner: &owner}); err != nil {
		return Receipt{}, err
	}
	j, _ = tx.state.owned(principal, id)
	return receipt(now, j.Reservation), nil
}

func (s Store) CheckOwner(ctx context.Context, principal string, owner Ownership, policyPath, archive string) (OwnershipDecision, error) {
	if !identifier.MatchString(principal) || !owner.valid() {
		return OwnershipDecision{}, failure("invalid_arguments", "invalid principal or ownership tuple")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return OwnershipDecision{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return OwnershipDecision{}, err
	}
	j, err := tx.state.owned(principal, owner.ReservationID)
	if err != nil {
		return OwnershipDecision{}, err
	}
	if err = liveClaim(j, owner, tx.state.at); err != nil {
		return OwnershipDecision{}, err
	}
	now, end, err := s.recheckOwnershipInput(ctx, tx, j, policyPath, archive)
	if err != nil {
		return OwnershipDecision{}, err
	}
	if err = liveClaim(j, owner, now); err != nil {
		return OwnershipDecision{}, err
	}
	if err = cancelled(ctx); err != nil {
		return OwnershipDecision{}, err
	}
	return OwnershipDecision{Mode: "local_ownership_check", OwnershipValid: true, CheckedAt: stamp(now), ValidUntil: stamp(end), Reservation: j.Reservation}, nil
}

// Recover closes accounting only. confirmedStopped is an operator assertion;
// this package cannot establish process death, VM teardown or external fencing.
func (s Store) Recover(ctx context.Context, principal string, owner Ownership, confirmedStopped bool) (RecoveryReceipt, error) {
	if !identifier.MatchString(principal) || !owner.valid() {
		return RecoveryReceipt{}, failure("invalid_arguments", "invalid principal or ownership tuple")
	}
	if !confirmedStopped {
		return RecoveryReceipt{}, failure("stop_confirmation_required", "recovery requires an explicit stopped-worker assertion; no process termination is performed")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return RecoveryReceipt{}, err
	}
	defer tx.close()
	if err = tx.observe(ctx, s.now()); err != nil {
		return RecoveryReceipt{}, err
	}
	j, err := tx.state.owned(principal, owner.ReservationID)
	if err != nil {
		return RecoveryReceipt{}, err
	}
	if !owner.matches(j.Reservation) || (j.Status != "claimed" && j.Status != "recovered") {
		return RecoveryReceipt{}, ownerConflict()
	}
	if j.Status != "recovered" {
		if err = tx.append(ctx, event{Kind: "recover", At: stamp(tx.state.at), ID: j.ID, Owner: &owner, Stopped: true}); err != nil {
			return RecoveryReceipt{}, err
		}
		j, _ = tx.state.owned(principal, j.ID)
	}
	return RecoveryReceipt{Receipt: receipt(tx.state.at, j.Reservation)}, nil
}

// RecoveryStatus is a read-only verified view, including when appends are full or
// the observed clock regresses. It never repairs data or grants ownership.
func (s Store) RecoveryStatus(ctx context.Context, principal string) (RecoveryReport, error) {
	if !identifier.MatchString(principal) {
		return RecoveryReport{}, failure("invalid_arguments", "invalid principal")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return RecoveryReport{}, err
	}
	defer tx.close()
	now := s.now()
	if now.IsZero() {
		return RecoveryReport{}, failure("invalid_arguments", "clock must be set")
	}
	report := RecoveryReport{Mode: "local_recovery_inspection", JournalSequence: tx.state.sequence, JournalSHA256: tx.state.hash, RecordedAt: stamp(tx.state.at), ObservedAt: stamp(now), ClockRollback: now.Before(tx.state.at), Reservations: []Reservation{}}
	viewAt := now
	if report.ClockRollback {
		viewAt = tx.state.at
	}
	for _, j := range tx.state.jobs {
		if j.Principal == principal {
			report.Reservations = append(report.Reservations, j.current(viewAt))
		}
	}
	if !tx.identitiesUnchanged() {
		return RecoveryReport{}, stateIO()
	}
	if err = cancelled(ctx); err != nil {
		return RecoveryReport{}, err
	}
	return report, nil
}
