// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
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
	"github.com/snapcore/snapd/interfaces/apparmor"
	"github.com/snapcore/snapd/snap"
)

const hardwareControlSummary = `allows control of system hardware`

const hardwareControlBaseDeclarationSlots = `
  hwmon-control:
    allow-installation:
      slot-snap-type:
        - core
    deny-auto-connection: true
`

const hardwareControlConnectedPlugAppArmor = `
# Description: This interface allows for controlling hardware and platform
# devices connected to the system.
# This is reserved because it allows potentially disruptive operations and
# access to devices which may contain sensitive information.

# files in /sys pertaining to hardware (eg, 'lspci -A linux-sysfs')
/sys/{block,bus,class,devices,firmware}/{,**} rw,
`

type hwmonControlInterface struct {
	commonInterface
}

func (iface *hwmonControlInterface) BeforePreparePlug(plug *snap.PlugInfo) error {
	attrs := []
	for attr := range plug.Attrs {

		if attr != "channel" {
			return fmt.Errorf("Cannot add plug %s: unknown attribute, %s", iface.name, attr)
		}
	}
	if len(plug.Attrs) == 0 {
		return nil
	}
	if attrs == ["channel"] {
		channels, ok := plug.Attrs["channel"].([]interface{})
		if !ok {
			return fmt.Errorf("cannot add %s plug: %q must be unset, or a list of strings", iface.name, att)
		}
	}
}

func (iface *hwmonControlInterface) AppArmorConnectedPlug(spec *apparmor.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	var channel []interface{}
	_ = plug.Attr("channel", &channels)

	errPrefix := fmt.Sprintf(`cannot connect plug %s: `, plug.Name())
	buf := bytes.NewBufferString(iface.apparmorHeader)
	if err := allowChannelAccess(buf, channels); err != nil {
		return fmt.Errorf("%s%v", errPrefix, err)
	}
	spec.AddSnippet(buf.String())

	return nil
}

func init() {
	registerIface(&hwmonControlInterface{
		name:                  "hwmon-control",
		summary:               hardwareControlSummary,
		implicitOnCore:        true,
		implicitOnClassic:     true,
		baseDeclarationSlots:  hardwareControlBaseDeclarationSlots,
		connectedPlugAppArmor: hardwareControlConnectedPlugAppArmor,
	})
}
