package report

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"
)

const contractActivity = "../../contracts/kern.activity.v1.json"

// Mirror of the kern-ui test: what the real reporter puts on the wire must equal the
// published fixture.
func TestActivityReporterEmitsTheFixture(t *testing.T) {
	want := fixture(t, contractActivity)

	s := newSink(t, http.StatusAccepted)
	r := NewActivityReporter(s.server.URL)
	r.now = func() time.Time { return time.Date(2026, 7, 26, 12, 0, 1, 0, time.UTC) }

	r.Report(context.Background(), "a23ead5373d9b746", "hello", "greet", true)
	r.Flush()

	if got := s.last(); !reflect.DeepEqual(got, want) {
		t.Errorf("the emitted signal has drifted from the published contract.\n got: %#v\nwant: %#v", got, want)
	}
}

func TestActivityContractFieldNames(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewActivityReporter(s.server.URL)
	r.now = fixedNow

	r.Report(context.Background(), "r1", "g", "n", true)
	r.Flush()

	for _, key := range []string{"run_id", "graph", "node_id", "generating", "at"} {
		if _, ok := s.last()[key]; !ok {
			t.Errorf("field %q is missing from the payload", key)
		}
	}
}

// The whole point of reporting off the run's thread: an agent must not wait on the sink
// before it may start working.
func TestReportDoesNotBlockOnASlowSink(t *testing.T) {
	blocked := make(chan struct{})
	s := newSlowSink(t, blocked)
	r := NewActivityReporter(s.URL)
	r.now = fixedNow

	done := make(chan struct{})
	go func() {
		r.Report(context.Background(), "r1", "g", "n", true)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Report blocked on the sink")
	}
	close(blocked)
	r.Flush()
}

// Fire-and-forget must not mean fire-and-lose: the command flushes before it exits, or the
// last signal of a run — the one that turns the beacon off — dies with the process.
func TestFlushWaitsForSignalsInFlight(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewActivityReporter(s.server.URL)
	r.now = fixedNow

	for i := 0; i < 5; i++ {
		r.Report(context.Background(), "r1", "g", "n", i%2 == 0)
	}
	r.Flush()

	if got := s.count(); got != 5 {
		t.Errorf("the sink received %d signals, want 5 — Flush returned too early", got)
	}
}

// A run is usually already cancelled by the time its last agent stops. Reporting the stop
// on the run's context would mean never reporting it, and the beacon would stay lit.
func TestTheStopIsReportedEvenOnACancelledRun(t *testing.T) {
	s := newSink(t, http.StatusAccepted)
	r := NewActivityReporter(s.server.URL)
	r.now = fixedNow

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r.Report(ctx, "r1", "g", "n", false)
	r.Flush()

	if s.count() == 0 {
		t.Error("nothing was reported for a cancelled run")
	}
}

func TestActivityReporterIsDisabledWithoutAURL(t *testing.T) {
	r := NewActivityReporter("")

	if r.Enabled() {
		t.Error("a reporter with no URL reports itself enabled")
	}
	// Must be a harmless no-op rather than a panic or a wasted goroutine.
	r.Report(context.Background(), "r1", "g", "n", true)
	r.Flush()
}
