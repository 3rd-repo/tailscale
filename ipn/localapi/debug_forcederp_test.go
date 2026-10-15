// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !ts_omit_debug

package localapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeDebugForceDERP(t *testing.T) {
	b := newTestLocalBackend(t)
	h := handlerForTest(t, &Handler{
		PermitWrite: true,
		b:           b,
	})

	// 1. Initial query: should be false
	req := httptest.NewRequest("POST", "http://local-tailscaled.sock/localapi/v0/debug?action=force-derp", nil)
	resp := httptest.NewRecorder()
	h.serveDebug(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("query status = %d, want 200; body: %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "false" {
		t.Fatalf("query body = %q, want false", resp.Body.String())
	}

	// 2. Set to true
	req = httptest.NewRequest("POST", "http://local-tailscaled.sock/localapi/v0/debug?action=force-derp", strings.NewReader("true"))
	resp = httptest.NewRecorder()
	h.serveDebug(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("set status = %d, want 200; body: %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "true" {
		t.Fatalf("set body = %q, want true", resp.Body.String())
	}
	if !b.DebugForceDERP() {
		t.Fatalf("b.DebugForceDERP() = false, want true")
	}

	// 3. Set to false
	req = httptest.NewRequest("POST", "http://local-tailscaled.sock/localapi/v0/debug?action=force-derp", strings.NewReader("false"))
	resp = httptest.NewRecorder()
	h.serveDebug(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("unset status = %d, want 200; body: %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "false" {
		t.Fatalf("unset body = %q, want false", resp.Body.String())
	}
	if b.DebugForceDERP() {
		t.Fatalf("b.DebugForceDERP() = true, want false")
	}
}
