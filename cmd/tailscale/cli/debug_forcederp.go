// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/peterbourgon/ff/v3/ffcli"
	"tailscale.com/client/local"
)

var debugForceDERPCmd = &ffcli.Command{
	Name:       "force-derp",
	ShortUsage: "tailscale debug force-derp [true|false]",
	Exec:       runForceDERP,
	ShortHelp:  "Configure whether to force relaying via DERP",
}

func runForceDERP(ctx context.Context, args []string) error {
	switch len(args) {
	case 0:
		v, err := local.GetDebugResultJSON[bool](ctx, &localClient, "force-derp")
		if err != nil {
			return fmt.Errorf("failed to get force-derp status: %w", err)
		}
		printf("%v\n", v)
		return nil
	case 1:
		b, err := strconv.ParseBool(args[0])
		if err != nil {
			return fmt.Errorf("invalid boolean argument %q (expected true or false): %w", args[0], err)
		}
		body, err := json.Marshal(b)
		if err != nil {
			return err
		}
		if err := localClient.DebugActionBody(ctx, "force-derp", bytes.NewReader(body)); err != nil {
			return fmt.Errorf("failed to set force-derp: %w", err)
		}
		return nil
	default:
		return errors.New("expected at most one boolean argument (true or false)")
	}
}
