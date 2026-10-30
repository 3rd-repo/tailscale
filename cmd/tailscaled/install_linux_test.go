// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestBuildExecStart(t *testing.T) {
	tests := []struct {
		name      string
		targetBin string
		args      []string
		want      string
	}{
		{
			name:      "default-empty-args",
			targetBin: "/usr/local/bin/tailscaled",
			args:      nil,
			want:      "/usr/local/bin/tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/run/tailscale/tailscaled.sock --port=${PORT} $FLAGS",
		},
		{
			name:      "custom-statedir",
			targetBin: "/usr/local/bin/tailscaled",
			args:      []string{"-statedir=/data/tailscale"},
			want:      "/usr/local/bin/tailscaled -statedir=/data/tailscale --socket=/run/tailscale/tailscaled.sock",
		},
		{
			name:      "custom-state-and-socket",
			targetBin: "/usr/sbin/tailscaled",
			args:      []string{"--state=/var/lib/tailscale/tailscaled.state", "--socket=/tmp/tailscaled.sock"},
			want:      "/usr/sbin/tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/tmp/tailscaled.sock",
		},
		{
			name:      "args-with-spaces",
			targetBin: "/usr/local/bin/tailscaled",
			args:      []string{"-statedir=/mnt/my data/tailscale"},
			want:      `/usr/local/bin/tailscaled "-statedir=/mnt/my data/tailscale" --socket=/run/tailscale/tailscaled.sock`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExecStart(tt.targetBin, tt.args)
			if got != tt.want {
				t.Errorf("buildExecStart(%q, %v) = %q, want %q", tt.targetBin, tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildSystemdUnit(t *testing.T) {
	unit := buildSystemdUnit("/usr/local/bin/tailscaled", []string{"-statedir=/var/lib/tailscale"})
	for _, expected := range []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"ExecStart=/usr/local/bin/tailscaled -statedir=/var/lib/tailscale --socket=/run/tailscale/tailscaled.sock",
		"ExecStopPost=/usr/local/bin/tailscaled --cleanup",
		"Type=notify",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, expected) {
			t.Errorf("buildSystemdUnit missing %q; got:\n%s", expected, unit)
		}
	}
}
