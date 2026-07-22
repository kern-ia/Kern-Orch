package checkpoint

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yoann/kern-orch/internal/graph"
)

func TestGraphPathRoundTrips(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "gp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	err = st.Save(ctx, Record{
		RunID: "r1", Step: 1, Frontier: []string{"b"},
		State: graph.NewState(), Status: StatusRunning,
		GraphPath: "/abs/path/to/graph.yaml",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec, ok, err := st.Latest(ctx, "r1")
	if err != nil || !ok {
		t.Fatalf("Latest: ok=%v err=%v", ok, err)
	}
	if rec.GraphPath != "/abs/path/to/graph.yaml" {
		t.Fatalf("GraphPath = %q; want the saved absolute path", rec.GraphPath)
	}
}
