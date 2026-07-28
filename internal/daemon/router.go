// Package daemon is the HTTP transport for running kern-orch as a long-lived service:
// routing, authentication, and JSON in and out. It knows nothing about graphs, checkpoints
// or reporters — that is the Runner it is handed, so this package is testable with a fake
// and stays a single responsibility (spec: routing, not orchestration).
package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/yoann/kern-orch/internal/checkpoint"
)

// ErrUnknownRun is returned by ResumeRun when no checkpoint exists for the given id, so the
// router can answer 404 rather than guessing from an opaque error.
var ErrUnknownRun = errors.New("daemon: unknown run")

// Runner is what the daemon needs from the orchestration side. internal/cmd implements it,
// using the same engine wiring the CLI's `run` and `resume` commands use — this package
// never touches graph, report or checkpoint construction directly.
type Runner interface {
	// StartRun launches graphPath and returns its run id immediately. An error here is
	// synchronous and means the run never started at all — a bad path, a graph that fails
	// to load — so a caller learns about it rather than polling for a run that will never
	// appear.
	StartRun(ctx context.Context, graphPath string) (runID string, err error)

	// ResumeRun continues runID from its last checkpoint in the background. It returns
	// ErrUnknownRun when no checkpoint exists.
	ResumeRun(ctx context.Context, runID string) error

	ListRuns(ctx context.Context) ([]checkpoint.Summary, error)
	GetRun(ctx context.Context, runID string) (checkpoint.Record, bool, error)
}

// NewRouter builds the daemon's HTTP handler. An empty token leaves every endpoint open,
// which is the local-development case; the binary refuses to bind a public address without
// one, mirroring the same rule kern-ui enforces for the same reason.
func NewRouter(runner Runner, token string) http.Handler {
	s := &server{runner: runner, token: token}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /api/v1/runs", s.auth(s.handleStartRun))
	mux.HandleFunc("GET /api/v1/runs", s.auth(s.handleListRuns))
	mux.HandleFunc("GET /api/v1/runs/{id}", s.auth(s.handleGetRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/resume", s.auth(s.handleResumeRun))
	return mux
}

type server struct {
	runner Runner
	token  string
}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			next(w, r)
			return
		}
		presented := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if presented == "" || subtle.ConstantTimeCompare([]byte(s.token), []byte(presented)) != 1 {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r)
	}
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Graph string `json:"graph"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"graph\":\"<path>\"}")
		return
	}
	if strings.TrimSpace(body.Graph) == "" {
		writeError(w, http.StatusBadRequest, "graph is required")
		return
	}

	runID, err := s.runner.StartRun(r.Context(), body.Graph)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func (s *server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.runner.ListRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, ok, err := s.runner.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "unknown run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	err := s.runner.ResumeRun(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, ErrUnknownRun):
		writeError(w, http.StatusNotFound, "unknown run")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "resuming"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
