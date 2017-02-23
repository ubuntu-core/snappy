// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package builtin

import (
	"bytes"
	"fmt"

	"github.com/snapcore/snapd/interfaces"
)

const maliitInputMethodPermanentSlotAppArmor = `
# Description: Allow operating as a maliit server.

# DBus accesses
#include <abstractions/dbus-session>

/usr/share/glib-2.0/schemas/ r,

# maliit uses peer-to-peer dbus over a unix socket after address negotiation
# each application has its own one-to-one communication channel with the maliit
# server, over which all further communication happens.
unix (bind, listen, accept) type=stream addr="@/tmp/maliit-server/dbus-*",
`

const maliitInputMethodConnectedSlotAppArmor = `
# Provides the maliit address service which assigns an individual unix socket
# to each application
dbus (receive)
    bus=session
    interface="org.maliit.Server.Address"
    path=/org/maliit/server/address
    peer=(label=###PLUG_SECURITY_TAGS###),

# provide access to the peer-to-peer dbus socket assigned by the address service
unix (receive, send) type=stream addr="@/tmp/maliit-server/dbus-*" peer=(label=###PLUG_SECURITY_TAGS###),
`

const maliitInputMethodConnectedPlugAppArmor = `
# Description: Allow applications to connect to a maliit socket
# Usage: common

#include <abstractions/dbus-session>

# Allow applications to communicate with the maliit address service
# which assigns an individual unix socket for all further communication
# to happen over.
dbus (send)
    bus=session
    interface="org.maliit.Server.Address"
    path=/org/maliit/server/address
    peer=(label=###SLOT_SECURITY_TAGS###),

# provide access to the peer-to-peer dbus socket assigned by the address service
unix (send, receive, connect) type=stream addr=none peer=(label=###SLOT_SECURITY_TAGS###, addr="@/tmp/maliit-server/dbus-*"),
`

const maliitInputMethodPermanentSlotSecComp = `
recv
recvfrom
recvmsg
send
sendto
sendmsg
listen
accept
accept4
`

const maliitInputMethodConnectedPlugSecComp = `
recv
recvfrom
recvmsg
send
sendto
sendmsg
`

type MaliitInputMethodInterface struct{}

func (iface *MaliitInputMethodInterface) Name() string {
	return "maliit-input-method"
}

func (iface *MaliitInputMethodInterface) PermanentPlugSnippet(plug *interfaces.Plug, securitySystem interfaces.SecuritySystem) ([]byte, error) {
	return nil, nil
}

func (iface *MaliitInputMethodInterface) ConnectedPlugSnippet(plug *interfaces.Plug, slot *interfaces.Slot, securitySystem interfaces.SecuritySystem) ([]byte, error) {
	switch securitySystem {
	case interfaces.SecurityAppArmor:
		old := []byte("###SLOT_SECURITY_TAGS###")
		new := slotAppLabelExpr(slot)
		snippet := bytes.Replace([]byte(maliitInputMethodConnectedPlugAppArmor), old, new, -1)
		return snippet, nil
	case interfaces.SecuritySecComp:
		return []byte(maliitInputMethodConnectedPlugSecComp), nil
	}
	return nil, nil
}

func (iface *MaliitInputMethodInterface) PermanentSlotSnippet(slot *interfaces.Slot, securitySystem interfaces.SecuritySystem) ([]byte, error) {
	switch securitySystem {
	case interfaces.SecurityAppArmor:
		return []byte(maliitInputMethodPermanentSlotAppArmor), nil
	case interfaces.SecuritySecComp:
		return []byte(maliitInputMethodPermanentSlotSecComp), nil
	}
	return nil, nil
}

func (iface *MaliitInputMethodInterface) ConnectedSlotSnippet(plug *interfaces.Plug, slot *interfaces.Slot, securitySystem interfaces.SecuritySystem) ([]byte, error) {
	switch securitySystem {
	case interfaces.SecurityAppArmor:
		old := []byte("###PLUG_SECURITY_TAGS###")
		new := plugAppLabelExpr(plug)
		snippet := bytes.Replace([]byte(maliitInputMethodConnectedSlotAppArmor), old, new, -1)
		return snippet, nil
	}
	return nil, nil
}

func (iface *MaliitInputMethodInterface) SanitizePlug(slot *interfaces.Plug) error {
    if iface.Name() != slot.Interface {
        panic(fmt.Sprintf("plug is not of interface %q", iface))
    }
    return nil
}

func (iface *MaliitInputMethodInterface) SanitizeSlot(slot *interfaces.Slot) error {
    if iface.Name() != slot.Interface {
        panic(fmt.Sprintf("slot is not of interface %q", iface))
    }
    return nil
}

func (iface *MaliitInputMethodInterface) AutoConnect(*interfaces.Plug, *interfaces.Slot) bool {
	// allow what declarations allowed
	return true
}
