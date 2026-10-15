// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package ipnlocal

// DebugSetForceDERP controls whether magicsock is forced to only use DERP relay.
func (b *LocalBackend) DebugSetForceDERP(v bool) {
	if mc := b.MagicConn(); mc != nil {
		mc.SetForceDERP(v)
	}
}

// DebugForceDERP reports whether magicsock is forced to only use DERP relay.
func (b *LocalBackend) DebugForceDERP() bool {
	if mc := b.MagicConn(); mc != nil {
		return mc.ForceDERP()
	}
	return false
}
