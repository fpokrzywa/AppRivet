package admission

import "time"

func (s *journalState) applyOwnership(e event, now time.Time) error {
	if s.sequence == 0 || e.Config != nil || e.Job != nil || e.Owner == nil || !e.Owner.valid() || e.Owner.ReservationID != e.ID {
		return corrupt()
	}
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.ID != e.ID {
			continue
		}
		if e.Kind == "claim" {
			if e.Stopped || j.current(now).Status != "reserved" || e.Owner.Fence != e.Sequence {
				return corrupt()
			}
			for _, other := range s.jobs {
				if other.ClaimID == e.Owner.ClaimID {
					return corrupt()
				}
			}
			j.Status = "claimed"
			j.WorkerID = e.Owner.WorkerID
			j.ClaimID = e.Owner.ClaimID
			j.Fence = e.Owner.Fence
			j.ClaimedAt = e.At
		} else {
			if !e.Stopped || j.Status != "claimed" || !e.Owner.matches(j.Reservation) {
				return corrupt()
			}
			j.Status = "recovered"
		}
		return nil
	}
	return corrupt()
}
