// Package graph is the core execution engine: shared State, Nodes, and (later)
// Edges and routing. Node execution is deliberately kept independent of routing.
package graph

import "encoding/json"

// State is the shared, mutable object threaded along the graph and mutated by each
// node. It is serializable so the checkpoint store can persist it per step.
//
// State is not safe for concurrent mutation: for fan-out, the engine gives each
// parallel branch a Clone and Merges the results back on a single goroutine.
type State struct {
	data map[string]any
	// Step is the number of nodes executed so far in the run; used for checkpoint ordering.
	Step int
}

// NewState returns an empty state.
func NewState() *State {
	return &State{data: make(map[string]any)}
}

// Get returns the value for key and whether it was present.
func (s *State) Get(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Set stores value under key.
func (s *State) Set(key string, value any) {
	s.data[key] = value
}

// Has reports whether key is present.
func (s *State) Has(key string) bool {
	_, ok := s.data[key]
	return ok
}

// Keys returns the keys currently held (order unspecified).
func (s *State) Keys() []string {
	ks := make([]string, 0, len(s.data))
	for k := range s.data {
		ks = append(ks, k)
	}
	return ks
}

// Clone returns a deep-enough copy for isolating a fan-out branch: the top-level map
// is copied so Set/Merge on the clone never touch the original. Values are copied by
// reference — nodes must not mutate shared reference values in place.
func (s *State) Clone() *State {
	c := &State{data: make(map[string]any, len(s.data)), Step: s.Step}
	for k, v := range s.data {
		c.data[k] = v
	}
	return c
}

// Merge overlays other's keys onto s, other winning on conflicts. Step is not merged.
func (s *State) Merge(other *State) {
	for k, v := range other.data {
		s.data[k] = v
	}
}

// MarshalJSON serializes the state as {"step":N,"data":{...}}.
func (s *State) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Step int            `json:"step"`
		Data map[string]any `json:"data"`
	}{Step: s.Step, Data: s.data})
}

// UnmarshalJSON restores a state produced by MarshalJSON.
func (s *State) UnmarshalJSON(b []byte) error {
	var raw struct {
		Step int            `json:"step"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw.Data == nil {
		raw.Data = make(map[string]any)
	}
	s.data = raw.Data
	s.Step = raw.Step
	return nil
}
