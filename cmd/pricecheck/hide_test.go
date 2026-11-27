package main

import (
	"testing"
	"time"
)

func TestNextHide(t *testing.T) {
	now := time.Now()
	at := now.Add(30 * time.Second)
	if got, hide := nextHide(at, now, false); hide || !got.Equal(at) {
		t.Errorf("still cursor: %v %v", got, hide)
	}
	got, hide := nextHide(at, now, true)
	if hide || !got.Equal(now.Add(hideGrace)) {
		t.Errorf("moved away: hide at %v, want in %v", got.Sub(now), hideGrace)
	}
	// Later ticks with the cursor still away keep that time, not push it.
	if again, _ := nextHide(got, now.Add(time.Second), true); !again.Equal(got) {
		t.Errorf("grace restarted: %v", again.Sub(now))
	}
	if _, hide := nextHide(got, now.Add(hideGrace+time.Millisecond), true); !hide {
		t.Error("not hidden after the grace")
	}
	if _, hide := nextHide(at, at.Add(time.Millisecond), false); !hide {
		t.Error("not hidden after the timeout")
	}
}
