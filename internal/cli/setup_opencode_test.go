package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureOpenCodeCreatesConfigAndEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	envPath := filepath.Join(dir, "opencode-env.sh")

	if err := configureOpenCode(configPath, envPath); err != nil {
		t.Fatalf("configureOpenCode: %v", err)
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	config := string(configBytes)
	if !strings.Contains(config, `"@devtheops/opencode-plugin-otel"`) {
		t.Fatalf("config missing plugin: %s", config)
	}

	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	env := string(envBytes)
	for _, want := range []string{
		"export OPENCODE_ENABLE_TELEMETRY=1",
		"export OPENCODE_OTLP_ENDPOINT=http://localhost:4318",
		"export OPENCODE_OTLP_PROTOCOL=http/protobuf",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("env missing %q: %s", want, env)
		}
	}
}

func TestConfigureOpenCodeIsIdempotentAndPreservesFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	envPath := filepath.Join(dir, "opencode-env.sh")
	initial := `{"$schema":"https://opencode.ai/config.json","model":"anthropic/claude-sonnet-4","plugin":["existing-plugin"]}`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}
	if err := configureOpenCode(configPath, envPath); err != nil {
		t.Fatalf("first configureOpenCode: %v", err)
	}
	if err := configureOpenCode(configPath, envPath); err != nil {
		t.Fatalf("second configureOpenCode: %v", err)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	config := string(configBytes)
	if strings.Count(config, "@devtheops/opencode-plugin-otel") != 1 {
		t.Fatalf("plugin not idempotent: %s", config)
	}
	if !strings.Contains(config, "existing-plugin") || !strings.Contains(config, "anthropic/claude-sonnet-4") {
		t.Fatalf("did not preserve existing fields: %s", config)
	}
}
