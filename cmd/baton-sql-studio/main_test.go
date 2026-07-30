package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLI_CompileGolden(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "compile", "../../pkg/studio/testdata/finance.spec.json").CombinedOutput()
	if err != nil {
		t.Fatalf("compile failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "resource_types:") {
		t.Fatalf("expected yaml output, got:\n%s", out)
	}
}
