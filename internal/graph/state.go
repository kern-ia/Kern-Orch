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
	// zones holds the non-default zone label per key (persistent keys are not stored).
	zones map[string]string
	// Step is the number of nodes executed so far in the run; used for checkpoint ordering.
	Step int
	// Frozen counts how many times the context was respawned via Freeze (observability).
	Frozen int
}

// NewState returns an empty state.
func NewState() *State {
	return &State{data: make(map[string]any), zones: make(map[string]string)}
}

// Get returns the value for key and whether it was present.
func (s *State) Get(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Set stores value under key in the default (persistent) zone.
func (s *State) Set(key string, value any) {
	s.data[key] = value
	delete(s.zones, key) // re-setting resets a key to the persistent zone
}

// SetZoned stores value under key, tagging it with a context zone. A key in
// ZoneEphemeral is dropped by the default carry-over on Freeze.
func (s *State) SetZoned(zone, key string, value any) {
	s.data[key] = value
	if zone == "" || zone == ZonePersistent {
		delete(s.zones, key)
		return
	}
	s.zones[key] = zone
}

// Zone returns the context zone of key (ZonePersistent when untagged/absent).
func (s *State) Zone(key string) string {
	if z, ok := s.zones[key]; ok {
		return z
	}
	return ZonePersistent
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

// Clone returns a deep-enough copy for isolating a fan-out branch: the top-level maps
// are copied so Set/Merge on the clone never touch the original. Values are copied by
// reference — nodes must not mutate shared reference values in place.
func (s *State) Clone() *State {
	c := &State{
		data:   make(map[string]any, len(s.data)),
		zones:  make(map[string]string, len(s.zones)),
		Step:   s.Step,
		Frozen: s.Frozen,
	}
	for k, v := range s.data {
		c.data[k] = v
	}
	for k, z := range s.zones {
		c.zones[k] = z
	}
	return c
}

// replaceWith makes s adopt other's contents wholesale (data, zones, Frozen). Unlike
// Merge it honors deletions, so a Freeze on a single-node frontier propagates. Step is
// taken from other (the engine advances it afterwards).
func (s *State) replaceWith(other *State) {
	s.data = other.data
	s.zones = other.zones
	s.Frozen = other.Frozen
	s.Step = other.Step
}

// Merge overlays other's keys (and their zone labels) onto s, other winning on
// conflicts. Step and Frozen are not merged.
func (s *State) Merge(other *State) {
	for k, v := range other.data {
		s.data[k] = v
		if z, ok := other.zones[k]; ok {
			s.zones[k] = z
		} else {
			delete(s.zones, k)
		}
	}
}

// MarshalJSON serializes the state as {"step":N,"frozen":N,"data":{...},"zones":{...}}.
func (s *State) MarshalJSON() ([]byte, error) {
	return json.Marshal(stateWire{Step: s.Step, Frozen: s.Frozen, Data: s.data, Zones: s.zones})
}

// UnmarshalJSON restores a state produced by MarshalJSON.
func (s *State) UnmarshalJSON(b []byte) error {
	var raw stateWire
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw.Data == nil {
		raw.Data = make(map[string]any)
	}
	if raw.Zones == nil {
		raw.Zones = make(map[string]string)
	}
	s.data = raw.Data
	s.zones = raw.Zones
	s.Step = raw.Step
	s.Frozen = raw.Frozen
	return nil
}

type stateWire struct {
	Step   int               `json:"step"`
	Frozen int               `json:"frozen"`
	Data   map[string]any    `json:"data"`
	Zones  map[string]string `json:"zones"`
}
