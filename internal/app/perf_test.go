package app

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestFrameTimes tells how long the window takes to draw each section, and
// to move the choice in it, in the repository $LAZIERGIT_PERF_REPO:
//
//	LAZIERGIT_PERF_REPO=~/src/go go test -run TestFrameTimes -v ./internal/app
func TestFrameTimes(t *testing.T) {
	dir := os.Getenv("LAZIERGIT_PERF_REPO")
	if dir == "" {
		t.Skip("LAZIERGIT_PERF_REPO not set")
	}
	a, tt, _ := testerFor(t, dir)
	frame := func() time.Duration {
		best := time.Hour
		for range 5 {
			start := time.Now()
			tt.Frame()
			best = min(best, time.Since(start))
		}
		return best
	}
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	for _, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9} {
		tt.Key(0, key)
		tt.Frame()
		tt.Frame() // details loaded
		still := frame()
		start := time.Now()
		tt.Key(0, ui.KeyJ)
		tt.Frame()
		step := time.Since(start)
		t.Logf("%-12s frame %6.1f ms, j %6.1f ms", a.section, ms(still), ms(step))
	}
}
