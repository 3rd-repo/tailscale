// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !ts_omit_debug

package localapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (h *Handler) serveDebugForceDERP(w http.ResponseWriter, r *http.Request) error {
	if r.Body != nil {
		var v bool
		if err := json.NewDecoder(r.Body).Decode(&v); err == nil {
			h.b.DebugSetForceDERP(v)
		} else if !errors.Is(err, io.EOF) {
			return err
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(h.b.DebugForceDERP())
}
