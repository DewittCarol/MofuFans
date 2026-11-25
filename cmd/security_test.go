package main

import (
	"bytes"
	"mofufans/internal/guard"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityCLIRequiresExclusiveOfflineRecovery(t *testing.T) {
	root := t.TempDir()
	g, err := guard.New(guard.Options{Directory: filepath.Join(root, "security"), Secret: strings.Repeat("s", 32)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.AddBan("192.0.2.1", 15); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"--state-directory", root, "--unban-ip", "192.0.2.1"}
	if securityCLI(args, &out) == nil {
		t.Fatal("live state recovered without lock")
	}
	g.Close()
	if err = securityCLI(args, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Removed 1") {
		t.Fatal(out.String())
	}
}
