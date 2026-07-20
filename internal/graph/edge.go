package graph

// RouteFunc decides, from the current state, which node(s) run next. It returns the
// successor node IDs; an empty/nil result means this branch terminates. Returning more
// than one ID is a fan-out. Routing is always Go-pure (no LLM) and lives outside the
// nodes themselves, keeping execution and routing independent.
type RouteFunc func(s *State) []string

// Static returns a RouteFunc that always yields the given IDs in order.
func Static(ids ...string) RouteFunc {
	fixed := append([]string(nil), ids...)
	return func(*State) []string { return fixed }
}

// Conditional adapts a decision function into a RouteFunc.
func Conditional(fn func(s *State) []string) RouteFunc {
	return fn
}

// Terminal is a RouteFunc that always ends the branch.
func Terminal() RouteFunc {
	return func(*State) []string { return nil }
}
