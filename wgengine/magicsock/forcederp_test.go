// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"tailscale.com/tailcfg"
	"tailscale.com/tstime/mono"
	"tailscale.com/types/key"
)

func TestForceDERP(t *testing.T) {
	c := newConn(t.Logf)

	if c.ForceDERP() {
		t.Fatal("expected ForceDERP initially false")
	}

	nodeKey := key.NewNode().Public()
	ep := &endpoint{
		c:         c,
		nodeID:    1,
		publicKey: nodeKey,
		bestAddr: addrQuality{
			epAddr: epAddr{ap: netip.MustParseAddrPort("192.168.1.100:1234")},
		},
		trustBestAddrUntil: mono.Now().Add(time.Hour),
		derpAddr:           netip.AddrPortFrom(tailcfg.DerpMagicIPAddr, 2),
	}

	c.mu.Lock()
	c.peerMap.upsertEndpoint(ep, key.DiscoPublic{}, false)
	c.mu.Unlock()

	// Enable force-derp
	c.SetForceDERP(true)
	if !c.ForceDERP() {
		t.Fatal("expected ForceDERP to be true after SetForceDERP(true)")
	}

	// Verify that existing bestAddr was cleared
	ep.mu.Lock()
	if ep.bestAddr.ap.IsValid() {
		t.Fatalf("expected bestAddr to be cleared, got %v", ep.bestAddr.ap)
	}
	ep.mu.Unlock()

	// Verify addrForSendLocked returns DERP only
	now := mono.Now()
	ep.mu.Lock()
	udpAddr, derpAddr, shouldPing := ep.addrForSendLocked(now)
	ep.mu.Unlock()

	if udpAddr.ap.IsValid() {
		t.Fatalf("addrForSendLocked returned valid udpAddr %v, want empty", udpAddr)
	}
	wantDerp := netip.AddrPortFrom(tailcfg.DerpMagicIPAddr, 2)
	if derpAddr != wantDerp {
		t.Fatalf("addrForSendLocked derpAddr = %v, want %v", derpAddr, wantDerp)
	}
	if shouldPing {
		t.Fatal("addrForSendLocked shouldPing = true, want false")
	}

	// Verify addrForPingSizeLocked returns DERP only
	ep.mu.Lock()
	udpAddr, derpAddr = ep.addrForPingSizeLocked(now, 100)
	ep.mu.Unlock()

	if udpAddr.ap.IsValid() {
		t.Fatalf("addrForPingSizeLocked returned valid udpAddr %v, want empty", udpAddr)
	}
	if derpAddr != wantDerp {
		t.Fatalf("addrForPingSizeLocked derpAddr = %v, want %v", derpAddr, wantDerp)
	}

	// Verify determineEndpoints returns nil endpoints when ForceDERP is true
	eps, err := c.determineEndpoints(context.Background())
	if err != nil {
		t.Fatalf("determineEndpoints error: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("determineEndpoints returned %d endpoints, want 0", len(eps))
	}

	// Verify UDP sending is blocked
	sent, err := c.sendUDP(netip.MustParseAddrPort("192.168.1.100:1234"), []byte("test"), false, false)
	if sent || err != errNoUDP {
		t.Fatalf("sendUDP returned sent=%v, err=%v; want sent=false, errNoUDP", sent, err)
	}

	sent, err = c.sendUDPBatch(epAddr{ap: netip.MustParseAddrPort("192.168.1.100:1234")}, [][]byte{[]byte("test")}, 0)
	if sent || err != errNoUDP {
		t.Fatalf("sendUDPBatch returned sent=%v, err=%v; want sent=false, errNoUDP", sent, err)
	}

	// Disable force-derp
	c.SetForceDERP(false)
	if c.ForceDERP() {
		t.Fatal("expected ForceDERP to be false after SetForceDERP(false)")
	}
}
