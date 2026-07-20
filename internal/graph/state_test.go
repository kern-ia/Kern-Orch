package graph

import (
	"encoding/json"
	"testing"
)

func TestStateSetGet(t *testing.T) {
	s := NewState()
	s.Set("count", 3)
	got, ok := s.Get("count")
	if !ok || got != 3 {
		t.Fatalf("Get(count) = %v, %v; want 3, true", got, ok)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get(missing) reported present")
	}
}

func TestStateCloneIsDeepIsolated(t *testing.T) {
	s := NewState()
	s.Set("k", "v")
	s.Step = 5
	c := s.Clone()
	c.Set("k", "changed")
	c.Step = 99
	if v, _ := s.Get("k"); v != "v" {
		t.Fatalf("clone mutation leaked into original: %v", v)
	}
	if s.Step != 5 {
		t.Fatalf("clone Step mutation leaked: %d", s.Step)
	}
}

func TestStateJSONRoundTrip(t *testing.T) {
	s := NewState()
	s.Set("name", "kern")
	s.Step = 2
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back State
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := back.Get("name"); v != "kern" {
		t.Fatalf("round-trip data lost: %v", v)
	}
	if back.Step != 2 {
		t.Fatalf("round-trip Step lost: %d", back.Step)
	}
}

func TestStateMergeOverlaysKeys(t *testing.T) {
	base := NewState()
	base.Set("a", 1)
	base.Set("b", 2)
	patch := NewState()
	patch.Set("b", 20)
	patch.Set("c", 3)
	base.Merge(patch)
	for k, want := range map[string]any{"a": 1, "b": 20, "c": 3} {
		if v, _ := base.Get(k); v != want {
			t.Fatalf("Merge %s = %v; want %v", k, v, want)
		}
	}
}
