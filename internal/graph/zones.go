package graph

// Context zones scope keys of the shared State so a long-running agent can keep its
// working context small: ephemeral scratch is separated from what must survive, and a
// Freeze respawns a fresh context ("gel = respawn contexte frais", cf. spec §Orchestration).
const (
	// ZonePersistent is the default zone; its keys survive a Freeze.
	ZonePersistent = "persistent"
	// ZoneEphemeral holds scratch context dropped by the default carry-over on Freeze.
	ZoneEphemeral = "ephemeral"
)

// CarryOver decides what survives a Freeze: given the current state it returns the
// key/values to keep in the fresh context. Returned keys land in the persistent zone.
type CarryOver func(s *State) map[string]any

// DefaultCarryOver keeps every persistent-zone key and drops the rest (e.g. ephemeral).
func DefaultCarryOver(s *State) map[string]any {
	out := make(map[string]any)
	for k, v := range s.data {
		if s.Zone(k) == ZonePersistent {
			out[k] = v
		}
	}
	return out
}

// Freeze respawns a fresh context: it replaces the state's contents with only what the
// carry-over keeps (all landing in the persistent zone), increments Frozen, and preserves
// Step. A nil carry uses DefaultCarryOver. This is how an agent avoids context bloat —
// break work into short tasks, freeze, and continue from a clean slate.
func (s *State) Freeze(carry CarryOver) {
	if carry == nil {
		carry = DefaultCarryOver
	}
	kept := carry(s)
	s.data = make(map[string]any, len(kept))
	s.zones = make(map[string]string)
	for k, v := range kept {
		s.data[k] = v
	}
	s.Frozen++
}
