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
)

// Config is the resolved runtime configuration.
type Config struct {
	SkillsDir    string
	CheckpointDB string
	AgentCLI     string // path to external LLM CLI; empty => use the deterministic stub
}

// FromEnv builds a Config from the environment, applying defaults for unset variables.
func FromEnv() Config {
	return Config{
		SkillsDir:    envOr(EnvSkillsDir, "skills"),
		CheckpointDB: envOr(EnvCheckpointDB, "./data/kern-orch.db"),
		AgentCLI:     os.Getenv(EnvAgentCLI),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
