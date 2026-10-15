// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package magicsock

// ForceDERP reports whether magicsock is forced to only use DERP relay.
func (c *Conn) ForceDERP() bool {
	return c.forceDERP.Load()
}

// SetForceDERP controls whether magicsock is forced to only use DERP relay.
// If true, direct UDP connections are disabled, active bestAddr mappings are
// cleared, and all peer communication is forced over DERP.
func (c *Conn) SetForceDERP(v bool) {
	c.logf("magicsock: [debug] force DERP set to: %v", v)
	c.forceDERP.Store(v)
	c.mu.Lock()
	if v {
		c.peerMap.forEachEndpoint(func(ep *endpoint) {
			ep.mu.Lock()
			ep.clearBestAddrLocked()
			ep.mu.Unlock()
		})
	}
	c.mu.Unlock()
	c.ReSTUN("force-derp")
}
