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
	"io"
	"net/http"
	"strings"

	"github.com/yoann/kern-orch/internal/checkpoint"
	"github.com/yoann/kern-orch/internal/tools"
)

// ErrUnknownRun is returned by ResumeRun when no checkpoint exists for the given id, so the
// router can answer 404 rather than guessing from an opaque error.
var ErrUnknownRun = errors.New("daemon: unknown run")

// ErrUnknownTool is returned by InvokeTool when no tool skill of that name is loaded.
var ErrUnknownTool = errors.New("daemon: unknown tool")

// ErrForbidden is returned by StopRun, Nudge and Decide when the caller's actor does not
// match the run's own requester — C6's write path is the one place on this surface where
// who is asking matters, not just what they ask.
var ErrForbidden = errors.New("daemon: not this run's requester")

// ErrUnknownNode is returned by Decide when no node of that id is currently awaiting a
// decision on the given run — either it never paused there, or it already got one.
var ErrUnknownNode = errors.New("daemon: unknown node")

// Runner is what the daemon needs from the orchestration side. internal/cmd implements it,
// using the same engine wiring the CLI's `run` and `resume` commands use — this package
// never touches graph, report or checkpoint construction directly.
type Runner interface {
	// StartRun launches graphPath and returns its run id immediately. An error here is
	// synchronous and means the run never started at all — a bad path, a graph that fails
	// to load — so a caller learns about it rather than polling for a run that will never
	// appear. requester names who asked; empty leaves the run open to any actor.
	StartRun(ctx context.Context, graphPath, requester string) (runID string, err error)

	// ResumeRun continues runID from its last checkpoint in the background. It returns
	// ErrUnknownRun when no checkpoint exists.
	ResumeRun(ctx context.Context, runID string) error

	ListRuns(ctx context.Context) ([]checkpoint.Summary, error)
	GetRun(ctx context.Context, runID string) (checkpoint.Record, bool, error)

	// StopRun cancels a live run. ErrUnknownRun if it never existed, ErrForbidden if actor
	// is not the run's requester.
	StopRun(ctx context.Context, runID, actor string) error

	// Nudge queues a state key/value for the next level of a live run to pick up. Same
	// error shape as StopRun.
	Nudge(ctx context.Context, runID, actor, key string, value any) error

	// Decide answers a pending approval node. ErrUnknownNode if no node of that id is
	// currently waiting on this run.
	Decide(ctx context.Context, runID, nodeID, actor, decision string) error

	// ListTools returns every invocable tool skill's spec — an Espace widget's catalogue.
	ListTools(ctx context.Context) ([]tools.Spec, error)

	// InvokeTool runs the named tool with input and returns its display value. It returns
	// ErrUnknownTool when no tool skill of that name is loaded.
	InvokeTool(ctx context.Context, name string, input map[string]any) (tools.Result, error)
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
	mux.HandleFunc("GET /api/v1/tools", s.auth(s.handleListTools))
	mux.HandleFunc("POST /api/v1/tools/{name}/invoke", s.auth(s.handleInvokeTool))
	mux.HandleFunc("POST /api/v1/runs/{id}/stop", s.auth(s.handleStopRun))
	mux.HandleFunc("POST /api/v1/runs/{id}/nudge", s.auth(s.handleNudge))
	mux.HandleFunc("POST /api/v1/runs/{id}/nodes/{node}/decide", s.auth(s.handleDecide))
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
		Graph     string `json:"graph"`
		Requester string `json:"requester"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"graph\":\"<path>\"}")
		return
	}
	if strings.TrimSpace(body.Graph) == "" {
		writeError(w, http.StatusBadRequest, "graph is required")
		return
	}

	runID, err := s.runner.StartRun(r.Context(), body.Graph, body.Requester)
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

func (s *server) handleListTools(w http.ResponseWriter, r *http.Request) {
	specs, err := s.runner.ListTools(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, specs)
}

func (s *server) handleInvokeTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input map[string]any `json:"input"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"input\":{...}}")
		return
	}

	result, err := s.runner.InvokeTool(r.Context(), r.PathValue("name"), body.Input)
	switch {
	case errors.Is(err, ErrUnknownTool):
		writeError(w, http.StatusNotFound, "unknown tool")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actor string `json:"actor"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"actor\":\"...\"}")
		return
	}

	err := s.runner.StopRun(r.Context(), r.PathValue("id"), body.Actor)
	switch {
	case errors.Is(err, ErrUnknownRun):
		writeError(w, http.StatusNotFound, "unknown run")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "not this run's requester")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	}
}

func (s *server) handleNudge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actor string `json:"actor"`
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"key\":\"...\",\"value\":...}")
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	err := s.runner.Nudge(r.Context(), r.PathValue("id"), body.Actor, body.Key, body.Value)
	switch {
	case errors.Is(err, ErrUnknownRun):
		writeError(w, http.StatusNotFound, "unknown run")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "not this run's requester")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
	}
}

func (s *server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actor    string `json:"actor"`
		Decision string `json:"decision"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: want {\"decision\":\"approve|refuse\"}")
		return
	}

	err := s.runner.Decide(r.Context(), r.PathValue("id"), r.PathValue("node"), body.Actor, body.Decision)
	switch {
	case errors.Is(err, ErrUnknownRun), errors.Is(err, ErrUnknownNode):
		writeError(w, http.StatusNotFound, "unknown run or node")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "not this run's requester")
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "decided"})
	}
}

// decodeBody decodes r's JSON body into v. A missing body is fine — v keeps its zero
// value, the same shape every steer endpoint already treats as "no actor given" or
// "empty run" — only a genuinely malformed body is an error.
func decodeBody(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
