// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"context"
	"testing"
)

func TestDebugForceDERPSubcommand(t *testing.T) {
	cmd := debugCmd()
	var found bool
	for _, sub := range cmd.Subcommands {
		if sub.Name == "force-derp" {
			found = true
			if sub.Exec == nil {
				t.Error("force-derp subcommand Exec is nil")
			}
			break
		}
	}
	if !found {
		t.Fatal("force-derp subcommand not found in debugCmd")
	}

	// Test argument validation
	err := runForceDERP(context.Background(), []string{"not-a-bool"})
	if err == nil {
		t.Fatal("expected error for invalid boolean argument")
	}

	err = runForceDERP(context.Background(), []string{"true", "extra"})
	if err == nil {
		t.Fatal("expected error for extra arguments")
	}
}
