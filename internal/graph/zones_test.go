package graph

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSetDefaultsToPersistentZone(t *testing.T) {
	s := NewState()
	s.Set("k", 1)
	if z := s.Zone("k"); z != ZonePersistent {
		t.Fatalf("Zone(k) = %q; want persistent", z)
	}
	if z := s.Zone("missing"); z != ZonePersistent {
		t.Fatalf("Zone(missing) = %q; want persistent default", z)
	}
}

func TestSetZonedTagsTheKey(t *testing.T) {
	s := NewState()
	s.SetZoned(ZoneEphemeral, "scratch", "tmp")
	if v, _ := s.Get("scratch"); v != "tmp" {
		t.Fatalf("Get(scratch) = %v; want tmp", v)
	}
	if z := s.Zone("scratch"); z != ZoneEphemeral {
		t.Fatalf("Zone(scratch) = %q; want ephemeral", z)
	}
}

func TestFreezeDropsEphemeralKeepsPersistent(t *testing.T) {
	s := NewState()
	s.Set("goal", "ship") // persistent by default
	s.SetZoned(ZoneEphemeral, "buf1", "a")
	s.SetZoned(ZoneEphemeral, "buf2", "b")
	s.Step = 7

	s.Freeze(nil) // default carry-over

	if v, _ := s.Get("goal"); v != "ship" {
		t.Fatalf("persistent key lost: %v", v)
	}
	if s.Has("buf1") || s.Has("buf2") {
		t.Fatalf("ephemeral keys survived freeze: %v", s.Keys())
	}
	if s.Step != 7 {
		t.Fatalf("Step should survive freeze, got %d", s.Step)
	}
	if s.Frozen != 1 {
		t.Fatalf("Frozen = %d; want 1", s.Frozen)
	}
	// After freeze, kept keys are back in the persistent zone.
	if z := s.Zone("goal"); z != ZonePersistent {
		t.Fatalf("kept key zone = %q; want persistent", z)
	}
}

func TestFreezeCustomCarryOver(t *testing.T) {
	s := NewState()
	s.Set("keepme", 1)
	s.Set("dropme", 2)
	// Custom carry-over keeps only "keepme".
	s.Freeze(func(st *State) map[string]any {
		v, _ := st.Get("keepme")
		return map[string]any{"keepme": v}
	})
	if !s.Has("keepme") || s.Has("dropme") {
		t.Fatalf("custom carry-over not applied: %v", s.Keys())
	}
}

func TestCloneAndMergePreserveZonesAndFrozen(t *testing.T) {
	s := NewState()
	s.SetZoned(ZoneEphemeral, "e", 1)
	s.Freeze(func(*State) map[string]any { return map[string]any{} }) // Frozen=1, empties
	s.SetZoned(ZoneEphemeral, "e2", 2)

	c := s.Clone()
	if c.Zone("e2") != ZoneEphemeral {
		t.Fatalf("clone lost zone label: %q", c.Zone("e2"))
	}
	if c.Frozen != 1 {
		t.Fatalf("clone lost Frozen: %d", c.Frozen)
	}

	dst := NewState()
	dst.Merge(c)
	if dst.Zone("e2") != ZoneEphemeral {
		t.Fatalf("merge lost zone label: %q", dst.Zone("e2"))
	}
}

func TestZonesSurviveJSONRoundTrip(t *testing.T) {
	s := NewState()
	s.Set("p", "keep")
	s.SetZoned(ZoneEphemeral, "e", "tmp")
	s.Frozen = 3
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Zone("e") != ZoneEphemeral {
		t.Fatalf("round-trip lost zone: %q", back.Zone("e"))
	}
	if back.Frozen != 3 {
		t.Fatalf("round-trip lost Frozen: %d", back.Frozen)
	}
}

// A freeze tool node, run inside the engine, respawns a fresh context downstream.
func TestFreezeInsideEngine(t *testing.T) {
	g := NewGraph()
	g.AddNode(NewToolNode("work", func(_ context.Context, s *State) error {
		s.Set("result", "kept")                 // persistent
		s.SetZoned(ZoneEphemeral, "scratch", 1) // should be dropped
		return nil
	}))
	g.AddNode(NewToolNode("freeze", func(_ context.Context, s *State) error {
		s.Freeze(nil)
		return nil
	}))
	g.AddNode(NewToolNode("after", func(_ context.Context, s *State) error {
		if s.Has("scratch") {
			t.Errorf("downstream still sees ephemeral scratch")
		}
		s.Set("saw_result", s.Has("result"))
		return nil
	}))
	g.SetEntry("work")
	g.AddEdge("work", Static("freeze"))
	g.AddEdge("freeze", Static("after"))

	s := NewState()
	if err := NewEngine(g).Run(context.Background(), s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := s.Get("saw_result"); v != true {
		t.Fatalf("persistent result did not survive respawn: %v", s.Keys())
	}
	if s.Frozen != 1 {
		t.Fatalf("Frozen = %d; want 1", s.Frozen)
	}
}
