package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvalCli_Main_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	inputPath := filepath.Join(tmpDir, "mock_chat.json")

	configYAML := `
generative_routing:
  enabled: true
  evaluators:
    - name: test-eval
      type: builtin
      threshold: 5
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	inputJSON := `
{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "1234567890"}
  ]
}
`
	if err := os.WriteFile(inputPath, []byte(inputJSON), 0644); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}

	// Save original args
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()

	os.Args = []string{
		"eval-cli",
		"-config", configPath,
		"-input", inputPath,
		"-evaluator", "test-eval",
	}

	// Will print strictly to stdout, but we just want to ensure it completes without fataling.
	main()
}
