package main

import (
	"sync"
	"time"

	"github.com/Mi7teR/exile-price/internal/i18n"
)

// stageText is the pending line for a step of a trade request.
func stageText(step string, wait time.Duration) string {
	switch {
	case wait > 0:
		return i18n.Tf("waiting for the trade limit · %d s…", int(wait.Seconds()+0.999))
	case step == "fetch":
		return i18n.T("reading listings…")
	default:
		return i18n.T("searching trade…")
	}
}

// stages passes the steps of one price to show until stop: fetches still
// running after a failed price must not cover its answer.
type stages struct {
	show func(text string)
	mu   sync.Mutex
	done bool
}

func (s *stages) report(step string, wait time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.show(stageText(step, wait))
	}
}

func (s *stages) stop() {
	s.mu.Lock()
	s.done = true
	s.mu.Unlock()
}
