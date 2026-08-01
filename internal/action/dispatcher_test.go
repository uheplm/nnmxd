package action

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeNiri struct {
	mu      sync.Mutex
	actions []string
}

func (f *fakeNiri) Action(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, name)
}

func (f *fakeNiri) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.actions))
	copy(out, f.actions)
	return out
}

type fakeLogid struct {
	available atomic.Bool

	mu            sync.Mutex
	setProfiles   []string
	cycleCalls    int
	setProfileErr error
	cycleErr      error
}

func (f *fakeLogid) Available() bool { return f.available.Load() }

func (f *fakeLogid) SetProfile(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setProfiles = append(f.setProfiles, name)
	return f.setProfileErr
}

func (f *fakeLogid) CycleProfile() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cycleCalls++
	return f.cycleErr
}

func (f *fakeLogid) calls() (sets []string, cycles int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.setProfiles))
	copy(out, f.setProfiles)
	return out, f.cycleCalls
}

// waitFor polls f until it returns true or times out. Used because logid
// dispatches run in their own goroutine.
func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestDispatchesPlainNameToNiri(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	d := New(n, l)

	d.Dispatch("focus-column-left")
	d.Dispatch("toggle-overview")

	got := n.got()
	want := []string{"focus-column-left", "toggle-overview"}
	if len(got) != len(want) {
		t.Fatalf("niri actions = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("niri[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	sets, cycles := l.calls()
	if len(sets) != 0 || cycles != 0 {
		t.Errorf("logid should not be touched; got sets=%v cycles=%d", sets, cycles)
	}
}

func TestEmptyNameIsNoop(t *testing.T) {
	n := &fakeNiri{}
	d := New(n, &fakeLogid{})

	d.Dispatch("")

	if got := n.got(); len(got) != 0 {
		t.Errorf("empty action should dispatch nothing, got %v", got)
	}
}

func TestLogidCycleProfile(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	l.available.Store(true)
	d := New(n, l)

	d.Dispatch("logid:cycle-profile")

	waitFor(t, func() bool {
		_, cycles := l.calls()
		return cycles == 1
	})
	if got := n.got(); len(got) != 0 {
		t.Errorf("niri should not be touched for logid:*, got %v", got)
	}
}

func TestLogidSetProfile(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	l.available.Store(true)
	d := New(n, l)

	d.Dispatch("logid:set-profile:FPS")

	waitFor(t, func() bool {
		sets, _ := l.calls()
		return len(sets) == 1 && sets[0] == "FPS"
	})
}

func TestLogidSetProfileWithEmptyNameWarnsAndDrops(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	l.available.Store(true)
	d := New(n, l)

	d.Dispatch("logid:set-profile:")

	// Give any spurious goroutine a moment to misbehave.
	time.Sleep(10 * time.Millisecond)
	sets, cycles := l.calls()
	if len(sets) != 0 || cycles != 0 {
		t.Errorf("empty profile name must not call backend; got sets=%v cycles=%d", sets, cycles)
	}
}

func TestLogidUnknownActionIsDropped(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	l.available.Store(true)
	d := New(n, l)

	d.Dispatch("logid:cycle-dpi")
	d.Dispatch("logid:toggle-smartshift")

	time.Sleep(10 * time.Millisecond)
	sets, cycles := l.calls()
	if len(sets) != 0 || cycles != 0 {
		t.Errorf("unknown logid actions must not call backend; got sets=%v cycles=%d", sets, cycles)
	}
	if got := n.got(); len(got) != 0 {
		t.Errorf("unknown logid actions must not fall through to niri, got %v", got)
	}
}

func TestLogidUnavailableDropsAction(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{}
	// available left at false
	d := New(n, l)

	d.Dispatch("logid:cycle-profile")
	d.Dispatch("logid:set-profile:FPS")

	time.Sleep(10 * time.Millisecond)
	sets, cycles := l.calls()
	if len(sets) != 0 || cycles != 0 {
		t.Errorf("unavailable logid must not be called; got sets=%v cycles=%d", sets, cycles)
	}
}

func TestLogidNilBackendDropsAction(t *testing.T) {
	n := &fakeNiri{}
	d := New(n, nil)

	d.Dispatch("logid:cycle-profile")

	time.Sleep(10 * time.Millisecond)
	if got := n.got(); len(got) != 0 {
		t.Errorf("nil logid backend must not fall through to niri, got %v", got)
	}
}

func TestBackendErrorDoesNotPanic(t *testing.T) {
	n := &fakeNiri{}
	l := &fakeLogid{
		setProfileErr: errors.New("boom"),
		cycleErr:      errors.New("boom"),
	}
	l.available.Store(true)
	d := New(n, l)

	d.Dispatch("logid:cycle-profile")
	d.Dispatch("logid:set-profile:FPS")

	waitFor(t, func() bool {
		sets, cycles := l.calls()
		return cycles == 1 && len(sets) == 1
	})
}

func TestRepeatedUnknownActionsWarnOnlyOnce(t *testing.T) {
	// We cannot capture log output without rewiring; instead, just verify
	// the dispatcher does not hang or panic on many repeats.
	n := &fakeNiri{}
	l := &fakeLogid{}
	l.available.Store(true)
	d := New(n, l)

	for range 100 {
		d.Dispatch("logid:cycle-dpi")
	}
}
