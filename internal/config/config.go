// Package config resolves harness configuration from environment variables with sane
// defaults. Precedence is: explicit CLI flags (applied by the caller) over env over
// these defaults.
package config

import "os"

// Environment variable names.
const (
	EnvSkillsDir    = "KERN_SKILLS_DIR"
	EnvCheckpointDB = "KERN_CHECKPOINT_DB"
	EnvAgentCLI     = "KERN_AGENT_CLI"
	// EnvStepReportURL points at an HTTP sink receiving one POST per completed graph
	// level. Unset means no reporting. The URL is the whole contract: kern-orch knows
	// nothing of the sink's route shape.
	EnvStepReportURL = "KERN_STEP_REPORT_URL"
	// EnvRegistryReportURL points at an HTTP sink receiving the whole skills catalogue.
	// It is a second variable rather than a route derived from EnvStepReportURL for the
	// reason stated above: the URL is the whole contract, so kern-orch must not invent a
	// sibling path on a host it knows nothing about.
	EnvRegistryReportURL = "KERN_REGISTRY_REPORT_URL"
	// EnvActivityReportURL points at an HTTP sink receiving one signal each time an agent
	// node starts and stops working. Same reasoning as the two above: its own URL, because
	// a sibling route cannot be invented for a host we know nothing about.
	EnvActivityReportURL = "KERN_ACTIVITY_REPORT_URL"
	// EnvSinkToken is the credential presented to every sink above. One secret for the three
	// URLs: they are three contracts to the same consumer, and asking an operator to manage
	// three secrets would mostly produce three copies of one.
	EnvSinkToken = "KERN_SINK_TOKEN"

	// EnvServeAddr is where `kern-orch serve` listens.
	EnvServeAddr = "KERN_ORCH_ADDR"
	// EnvServeToken is the bearer credential a caller of the daemon API must present.
	// Empty leaves the daemon open, which `serve` refuses on a public address — the same
	// rule kern-ui enforces on its own API, re-derived here rather than shared: the two
	// bricks depend on nothing of each other's.
	EnvServeToken = "KERN_ORCH_TOKEN"
)

// Config is the resolved runtime configuration.
type Config struct {
	SkillsDir    string
	CheckpointDB string
	AgentCLI     string // path to external LLM CLI; empty => use the deterministic stub
	// StepReportURL is an HTTP sink for step transitions; empty => no reporting.
	StepReportURL string
	// RegistryReportURL is an HTTP sink for the skills catalogue; empty => no publishing.
	RegistryReportURL string
	// ActivityReportURL is an HTTP sink for agent activity; empty => no reporting.
	ActivityReportURL string
	// SinkToken is presented to the sinks above; empty => reports travel anonymous.
	SinkToken string

	// ServeAddr is where `serve` listens.
	ServeAddr string
	// ServeToken is the credential the daemon API requires; empty => open (local dev only).
	ServeToken string
}

// FromEnv builds a Config from the environment, applying defaults for unset variables.
func FromEnv() Config {
	return Config{
		SkillsDir:         envOr(EnvSkillsDir, "skills"),
		CheckpointDB:      envOr(EnvCheckpointDB, "./data/kern-orch.db"),
		AgentCLI:          os.Getenv(EnvAgentCLI),
		StepReportURL:     os.Getenv(EnvStepReportURL),
		RegistryReportURL: os.Getenv(EnvRegistryReportURL),
		ActivityReportURL: os.Getenv(EnvActivityReportURL),
		SinkToken:         os.Getenv(EnvSinkToken),
		ServeAddr:         envOr(EnvServeAddr, "127.0.0.1:7070"),
		ServeToken:        os.Getenv(EnvServeToken),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
