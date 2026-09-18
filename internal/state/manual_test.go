package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

func newManual(t *testing.T) *Manual {
	t.Helper()
	m := LoadManual(filepath.Join(t.TempDir(), "manual-names.json"))

	return m
}

// sighting is the common case: the first tab in a workspace, which the
// resolver would name `dashboard`.
func sighting(current string) Sighting {
	return Sighting{TabID: "wE:t1", Current: current, Desired: "dashboard", Default: "1"}
}

func TestSightingFromDerivesTheDefaultLabel(t *testing.T) {
	// A tab nobody has named wears its position, so the third tab of a
	// workspace is `3` however high the ids around it have climbed.
	tab := TabFrom(herdr.TabInfo{TabID: "wE:t9", Label: "Important work"}, "work", 3, nil)

	got := SightingFrom(tab, "dashboard")
	want := Sighting{
		TabID:   "wE:t9",
		Current: "Important work",
		Desired: "dashboard",
		Default: "3",
	}

	if got != want {
		t.Errorf("sighting = %+v, want %+v", got, want)
	}
}

func TestTheFirstPollPreservesExplicitNames(t *testing.T) {
	m := LoadManual("")
	if m.Observe(sighting("1")) {
		t.Fatal("default label was locked")
	}

	if !m.Observe(Sighting{TabID: "wE:t2", Current: "Crew · task", Desired: "api", Default: "2"}) {
		t.Fatal("explicit startup label was not protected")
	}
}

func TestAppliedNamesRemainAutomaticAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual.json")
	m := LoadManual(path)
	m.Applied("wE:t1", "old title")
	m = LoadManual(path)
	m.Retain(map[string]string{"wE:t1": "old title"})

	if m.Observe(sighting("old title")) {
		t.Fatal("the plugin locked its own persisted label")
	}

	m = LoadManual(path)
	m.Retain(map[string]string{"wE:t1": "Crew · renamed offline"})

	if !m.Observe(sighting("Crew · renamed offline")) {
		t.Fatal("an offline explicit rename was overwritten")
	}
}

func TestDefaultManualPathsSeparateServers(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "one.sock"))

	one := DefaultManualPath()

	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "two.sock"))

	two := DefaultManualPath()
	if one == two || one == "" || two == "" {
		t.Fatal("server lock files overlap")
	}

	first := LoadManual(one)
	first.Observe(sighting("First crew"))

	second := LoadManual(two)
	second.Observe(sighting("Second crew"))
	second.Retain(map[string]string{})

	if !LoadManual(one).Locked("wE:t1") {
		t.Fatal("the second server removed the first server's lock")
	}
}

func TestATabTurningUpAlreadyNamedIsTheUsers(t *testing.T) {
	// The case that made this rule necessary: a tab created and named faster
	// than the next poll. Auto Title never saw it carrying its position, so
	// the name it carries is not Auto Title's.
	m := newManual(t)

	if !m.Observe(
		Sighting{TabID: "wE:t9", Current: "My thing", Desired: "dashboard", Default: "9"},
	) {
		t.Fatal("a tab that appeared already named was not read as the user's")
	}

	if !m.Locked("wE:t9") {
		t.Error("the tab is not locked")
	}
}

func TestATabTurningUpUnnamedIsNotTheUsers(t *testing.T) {
	// Herdr names a new tab after its position. Nobody has claimed this one.
	m := newManual(t)

	if m.Observe(Sighting{TabID: "wE:t9", Current: "9", Desired: "dashboard", Default: "9"}) {
		t.Error("an unnamed new tab was locked")
	}
}

func TestATabFallingBackToItsDefaultLabelIsNotTheUsers(t *testing.T) {
	// The default label is not only how a tab starts out: it comes back, and it
	// slides down for every tab that closes to the left. Locking there would
	// freeze the tab at a number for the rest of the session.
	m := newManual(t)
	m.Observe(sighting("1"))
	m.Applied("wE:t1", "dashboard")

	if m.Observe(sighting("1")) {
		t.Fatal("a tab back on its default label was read as the user's")
	}

	if m.Locked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestATabWhoseNameWasClearedIsNotTheUsers(t *testing.T) {
	// Clearing a tab's name empties its label rather than putting the position
	// back, so an empty label is Herdr's other way of saying nobody named it —
	// see docs/architecture/herdr-socket-api.md.
	m := newManual(t)
	m.Observe(sighting("1"))
	m.Applied("wE:t1", "dashboard")

	if m.Observe(sighting("")) {
		t.Fatal("a tab whose name was cleared was read as the user's")
	}

	if m.Locked("wE:t1") {
		t.Error("the tab is locked")
	}
}

func TestARenameByTheUserLocksTheTab(t *testing.T) {
	m := newManual(t)
	m.Observe(sighting("1"))
	m.Applied("wE:t1", "dashboard")

	if !m.Observe(sighting("Important work")) {
		t.Fatal("a label the plugin neither set nor wanted was not read as the user's")
	}

	if !m.Locked("wE:t1") {
		t.Error("the tab is not locked")
	}
}

func TestARenameByThePluginDoesNotLock(t *testing.T) {
	m := newManual(t)
	m.Observe(sighting("1"))
	m.Applied("wE:t1", "dashboard")

	if m.Observe(sighting("dashboard")) {
		t.Error("the plugin's own rename was read as the user's")
	}
}

func TestALabelThatHasNotMovedIsNobodysDoing(t *testing.T) {
	m := newManual(t)
	m.Observe(sighting("Important work"))

	// Same label on the next poll: nothing happened, whatever it says.
	if m.Observe(sighting("Important work")) {
		t.Error("an unchanged label was read as a rename")
	}
}

func TestALabelMatchingWhatWeWouldSetDoesNotLock(t *testing.T) {
	// Indistinguishable from the plugin's own work, and harmless either way.
	m := newManual(t)
	m.Observe(sighting("1"))

	if m.Observe(sighting("dashboard")) {
		t.Error("a label matching the resolved one locked the tab")
	}
}

func TestLocksSurviveAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Observe(sighting("1"))

	if !m.Observe(sighting("Important work")) {
		t.Fatal("the tab was not locked")
	}

	if !LoadManual(path).Locked("wE:t1") {
		t.Error("the lock did not survive a restart")
	}
}

func TestAReloadedLockIsReleasedWhenTheLabelMovedOn(t *testing.T) {
	// Herdr's tab ids belong to a session, so a stored wE:t1 may be an
	// unrelated tab by the time it is read back. Only the label makes it the
	// same tab.
	path := filepath.Join(t.TempDir(), "manual-names.json")

	m := LoadManual(path)
	m.Observe(sighting("1"))
	m.Observe(sighting("Important work"))

	reloaded := LoadManual(path)
	reloaded.Retain(map[string]string{"wE:t1": "2"})

	if reloaded.Locked("wE:t1") {
		t.Error("a lock was kept for a tab that no longer carries its name")
	}

	if LoadManual(path).Locked("wE:t1") {
		t.Error("the released lock was not written out")
	}
}

func TestRetainDropsTabsTheSessionNoLongerHolds(t *testing.T) {
	m := newManual(t)
	m.Observe(sighting("1"))
	m.Observe(sighting("Important work"))

	m.Retain(map[string]string{})

	if m.Locked("wE:t1") {
		t.Error("a closed tab is still locked")
	}

	// Its baseline went too, so a tab reusing the id starts clean and is
	// judged on what it carries rather than on what the old tab did.
	if m.Observe(Sighting{TabID: "wE:t1", Current: "1", Desired: "dashboard", Default: "1"}) {
		t.Error("an unnamed tab reusing the id was locked")
	}
}

func TestAnUnreadableStoreIsNotFatal(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "manual-names.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := LoadManual(path)
	if m.Locked("wE:t1") {
		t.Error("a corrupt store produced a lock")
	}
	// And it still works from there.
	m.Observe(sighting("1"))

	if !m.Observe(sighting("Important work")) {
		t.Error("locking stopped working after a corrupt store")
	}
}

func TestWithoutAPathLocksStayInMemory(t *testing.T) {
	m := LoadManual("")
	m.Observe(sighting("1"))

	if !m.Observe(sighting("Important work")) {
		t.Error("locking needs a file")
	}

	if !m.Locked("wE:t1") {
		t.Error("the lock was not kept")
	}
}
