package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State is the set of conditions a previous pass saw firing, keyed by
// Event.Key. It is what makes a watch edge-triggered without a process
// that stays running: the file it is read from and written back to is the
// memory a daemon would otherwise hold.
type State struct {
	Firing map[string]Event `json:"firing"`
}

// Record folds one pass's snapshot into the state to write back: every
// condition firing now, plus any the previous pass recorded that this one
// could not verify.
//
// The carry-forward is what keeps a network blip from re-breaking old
// news. A condition missing from a pass means one of two things — it
// cleared, or nobody could tell — and only the first should let it notify
// again. Without this, one unreachable remote or one expired gh session
// would erase the record and re-announce the whole backlog on the next
// pass that worked, which is how a notifier gets muted.
func Record(previous State, snap Snapshot) State {
	recorded := StateOf(snap.Firing)
	for _, key := range snap.Unverified {
		if was, known := previous.Firing[key]; known {
			if _, firing := recorded.Firing[key]; !firing {
				recorded.Firing[key] = was
			}
		}
	}
	return recorded
}

// StateOf records every condition in current as firing — the state to
// write back once this pass has delivered what it owed.
func StateOf(current []Event) State {
	firing := make(map[string]Event, len(current))
	for _, e := range current {
		firing[e.Key()] = e
	}
	return State{Firing: firing}
}

// Since returns the conditions in current that previous hadn't already
// seen firing, in current's order.
//
// A condition that has since cleared is simply absent from the state
// written back, which is what lets the same repo or worktree notify again
// the next time it goes stale or merges.
func Since(previous State, current []Event) []Event {
	var fired []Event
	for _, e := range current {
		if _, known := previous.Firing[e.Key()]; !known {
			fired = append(fired, e)
		}
	}
	return fired
}

// Drop forgets one condition, so the next pass treats it as new again. It
// is how a delivery that failed stays owed rather than being recorded as
// news already broken.
func (s State) Drop(key string) { delete(s.Firing, key) }

// LoadState reads what the previous pass recorded at path. A file that
// isn't there is a first run — every condition found is news — rather than
// an error.
//
// A file that is there but unreadable is an error, though. Treating it as
// a first run would re-notify the entire backlog on every pass, which is
// both wrong and the failure mode most likely to get the notifier turned
// off.
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{Firing: map[string]Event{}}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("parsing %s: %w (delete it to start over)", path, err)
	}
	if s.Firing == nil {
		s.Firing = map[string]Event{}
	}
	return s, nil
}

// SaveState writes s to path via a temporary file and a rename, so a pass
// killed mid-write leaves the previous state intact rather than a
// half-written file the next pass would refuse.
func SaveState(path string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
