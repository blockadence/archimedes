package dashboard

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// DefaultRefresh is how stale a reading is allowed to get before the
// dashboard retakes it. A reading costs one `gh pr list` per worktree row,
// so this is slow enough not to hammer the forge while an instance sits
// open on a second monitor, and quick enough that a PR merged in another
// window shows up without being asked for.
const DefaultRefresh = 30 * time.Second

// tickInterval is how often the loop wakes up. It's the resolution of the
// "updated Ns ago" clock in the header, not the refresh rate — a tick only
// takes a reading when one is actually due.
const tickInterval = time.Second

// snapshotMsg carries a finished reading back to the loop. The error rides
// along with the snapshot rather than replacing it, so a failed refresh
// leaves the last good reading on screen.
type snapshotMsg struct {
	snapshot Snapshot
	err      error
}

// tickMsg is the once-a-second heartbeat.
type tickMsg time.Time

// Model is the Bubble Tea loop behind the dashboard: it holds the latest
// reading, retakes it on a timer or on demand, and hands Render the frame
// to draw. It decides *when* to look at the instance and nothing about what
// it finds there.
type Model struct {
	opts Options
	// refresh is how old a reading may get before being retaken. Zero
	// turns automatic refreshing off, leaving "r" as the only way to take
	// a new one.
	refresh time.Duration
	// now is the clock, injectable so tests can age a reading without
	// waiting for one.
	now func() time.Time

	snapshot Snapshot
	loaded   bool
	// refreshing reports that a reading is in flight — which is also what
	// keeps a slow one from being started twice.
	refreshing bool
	// takenAt is when the displayed snapshot was collected; lastAttempt is
	// when a reading last came back either way. They differ after a
	// failure, and the difference matters: the header ages the reading
	// you're actually looking at, while the retry timer runs off the
	// attempt, so a broken instance is retried on the refresh interval
	// rather than once a tick.
	takenAt     time.Time
	lastAttempt time.Time
	err         error
	width       int
}

// New builds the loop for one instance. It starts out already refreshing,
// since Init's first act is to take a reading.
func New(opts Options, refresh time.Duration) Model {
	return Model{opts: opts, refresh: refresh, now: time.Now, refreshing: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.collect(), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m.startRefresh()
		}
		return m, nil

	case snapshotMsg:
		m.refreshing = false
		m.err = msg.err
		m.lastAttempt = m.clock()
		if msg.err == nil {
			m.snapshot = msg.snapshot
			m.loaded = true
			m.takenAt = m.lastAttempt
		}
		return m, nil

	case tickMsg:
		if !m.due() {
			return m, tick()
		}
		next, cmd := m.startRefresh()
		return next, tea.Batch(cmd, tick())
	}

	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView(Render(m.Frame()))
	v.AltScreen = true
	return v
}

// Frame is what the model would draw right now. Exported so the loop can be
// driven and asserted on without a terminal behind it.
func (m Model) Frame() Frame {
	f := Frame{
		Snapshot:   m.snapshot,
		Width:      m.width,
		Loaded:     m.loaded,
		Refreshing: m.refreshing,
		Err:        m.err,
	}
	if m.loaded {
		f.Age = m.clock().Sub(m.takenAt)
	}
	return f
}

// due reports whether the reading has aged past the refresh interval. A
// reading already in flight is never due — a `gh` call slower than the
// interval would otherwise queue up behind itself forever.
func (m Model) due() bool {
	return m.refresh > 0 && !m.refreshing && m.clock().Sub(m.lastAttempt) >= m.refresh
}

func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.refreshing {
		return m, nil
	}
	m.refreshing = true
	return m, m.collect()
}

// collect takes the next reading on Bubble Tea's own goroutine, off the UI
// loop: a reading shells out to `gh` once per worktree row, and the
// dashboard has to stay responsive to "q" throughout.
func (m Model) collect() tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		snapshot, err := Collect(opts)
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// clock reads the model's injected clock, tolerating a zero-valued Model so
// a caller that skipped New still gets a working one rather than a panic.
func (m Model) clock() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return m.now()
}
