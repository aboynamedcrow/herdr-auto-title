package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Manual remembers which tabs the user renamed by hand, so Auto Title stops
// naming them. A rename is not an event but a label that moved between two
// polls; see docs/architecture/manual-rename-protection.md.
type Manual struct {
	mu   sync.Mutex
	path string
	// seen is the label each tab carried when it was last looked at.
	seen map[string]string
	// locked is the label a tab carried when the user claimed it. The label,
	// not the id, is what makes a reloaded lock safe: Herdr reuses tab ids.
	locked  map[string]string
	applied map[string]string
}

// manualFile is the on-disk form: locks outlive the process because Herdr can
// restart a plugin mid-session.
type manualFile struct {
	Locked  map[string]string `json:"locked_tabs"`
	Applied map[string]string `json:"applied_tabs,omitempty"`
}

// LoadManual reads persisted locks from path. Anything unreadable yields an
// empty set: this is a convenience, not a reason to refuse to start.
func LoadManual(path string) *Manual {
	m := &Manual{
		path:    path,
		seen:    make(map[string]string),
		locked:  make(map[string]string),
		applied: make(map[string]string),
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return m
	}

	var stored manualFile
	if json.Unmarshal(raw, &stored) != nil {
		return m
	}

	maps.Copy(m.locked, stored.Locked)
	maps.Copy(m.applied, stored.Applied)
	maps.Copy(m.seen, stored.Applied)

	return m
}

// DefaultManualPath is where locks are kept when nothing says otherwise.
func DefaultManualPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	socket := os.Getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return ""
	}

	absolute, err := filepath.Abs(socket)
	if err != nil {
		return ""
	}

	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = canonical
	}

	name := fmt.Sprintf("manual-names-%x.json", sha256.Sum256([]byte(absolute)))

	return filepath.Join(dir, "herdr-auto-title", name)
}

// Locked reports whether the user has claimed this tab.
func (m *Manual) Locked(tabID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, locked := m.locked[tabID]

	return locked
}

// Sighting is what one poll saw of a tab: the label it carries, what the
// resolver would name it, and what Herdr names a tab nobody has claimed.
type Sighting struct {
	TabID   string
	Current string
	Desired string
	Default string
}

// SightingFrom is what a poll saw of a tab, given the name the resolver chose
// for it. What Herdr calls an unclaimed tab is derived here rather than by the
// caller: it is the tab's own position, which is this package's to know.
func SightingFrom(tab TabState, desired string) Sighting {
	return Sighting{
		TabID:   tab.ID,
		Current: tab.CurrentName,
		Desired: desired,
		Default: strconv.Itoa(tab.Position),
	}
}

// Observe records what a poll saw and reports whether the user put that label
// there.
func (m *Manual) Observe(s Sighting) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	previous, known := m.seen[s.TabID]
	m.seen[s.TabID] = s.Current

	switch {
	case s.Current == s.Desired:
		return false
	case s.Current == "", s.Current == s.Default:
		// Nobody has named it. A known tab can wear either: clearing a name
		// empties the label rather than putting the position back, and the
		// position itself slides down when a tab to the left closes.
		return false
	case known:
		if s.Current == previous {
			return false
		}
	}

	m.locked[s.TabID] = s.Current
	m.saveLocked()

	return true
}

// Applied records a label Auto Title has just set, so the next poll does not
// read its own work as the user's.
func (m *Manual) Applied(tabID, label string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.seen[tabID] = label
	m.applied[tabID] = label
	m.saveLocked()
}

// Retain drops everything about tabs the session no longer holds, and releases
// a lock whose tab now carries a different label — which is what stops a
// reloaded lock from claiming an unrelated tab that inherited its id.
func (m *Manual) Retain(live map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	changed := false

	for tabID, label := range m.locked {
		if current, alive := live[tabID]; !alive || current != label {
			delete(m.locked, tabID)

			changed = true
		}
	}

	for tabID, label := range m.applied {
		if current, alive := live[tabID]; !alive || current != label {
			delete(m.applied, tabID)
			delete(m.seen, tabID)

			changed = true
		}
	}

	for tabID := range m.seen {
		if _, alive := live[tabID]; !alive {
			delete(m.seen, tabID)
		}
	}

	if changed {
		m.saveLocked()
	}
}

// saveLocked writes the locks out through a temporary file, so a crash cannot
// leave a half-written one. The caller holds the mutex; failure is silent.
func (m *Manual) saveLocked() {
	if m.path == "" {
		return
	}

	if os.MkdirAll(filepath.Dir(m.path), 0o700) != nil {
		return
	}

	// encoding/json sorts map keys itself, so the file is diffable already.
	raw, err := json.MarshalIndent(manualFile{Locked: m.locked, Applied: m.applied}, "", "  ")
	if err != nil {
		return
	}

	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) != nil {
		return
	}

	if os.Rename(tmp, m.path) != nil {
		_ = os.Remove(tmp)
	}
}
