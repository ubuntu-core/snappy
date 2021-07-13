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
	"fmt"

	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/apparmor"
	"github.com/snapcore/snapd/interfaces/mount"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
)

// On systems where the slot is provided by an app snap, the cups interface is
// the companion interface to the cups-control interface. The design of these
// interfaces is based on the idea that the slot implementation (eg cupsd) is
// expected to query snapd to determine if the cups-control interface is
// connected or not for the peer client process and the print service will
// mediate admin functionality (ie, the rules in these interfaces allow
// connecting to the print service, but do not implement enforcement rules; it
// is up to the print service to provide enforcement).
const cupsSummary = `allows access to the CUPS socket for printing`

// cups is currently only available via a providing app snap and this interface
// assumes that the providing app snap also slots 'cups-control' (the current
// design allows the snap provider to slots both cups-control and cups or just
// cups-control (like with implicit classic or any slot provider without
// mediation patches), but not just cups).
const cupsBaseDeclarationSlots = `
  cups:
    allow-installation:
      slot-snap-type:
        - app
    deny-connection: true
    deny-auto-connection: true
`

const cupsConnectedPlugAppArmor = `
# Allow communicating with the cups server

#include <abstractions/cups-client>
/{,var/}run/cups/printcap r,

# allow talking to the snap cups socket
/var/cups/cups.sock rw,
`

type cupsInterface struct {
	commonInterface
}

func (iface *cupsInterface) AppArmorConnectedSlot(spec *apparmor.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	return nil
}

func (iface *cupsInterface) BeforePrepareSlot(slot *snap.SlotInfo) error {
	// verify that the snap has a cups-socket interface attribute, which is
	// needed to identify where to find the cups socket is located in the snap
	// providing the cups socket

	var cupsdSocketSource string
	if err := slot.Attr("cups-socket", &cupsdSocketSource); err != nil {
		return err
	}

	// TODO: we probably want to allow the empty slot for old cups snaps which
	//       won't have the slot and have a sensible default for those, or
	//       possibly just don't emit the mount if the attribute is not present

	if cupsdSocketSource == "" {
		return fmt.Errorf("cups slot must specify the location of the cups socket")
	}

	// TODO: should we do some checks on the cups-socket file location that it
	//       is a clean filepath, etc.

	return nil
}

func (iface *cupsInterface) AppArmorConnectedPlug(spec *apparmor.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	// add the base snippet
	spec.AddSnippet(cupsConnectedPlugAppArmor)

	// setup the snap-update-ns rules for bind mounting
	emit := spec.AddUpdateNSf

	var cupsdSocketSource string
	err := slot.Attr("cups-socket", &cupsdSocketSource)
	if err != nil {
		return fmt.Errorf("broken interface: %v", err)
	}

	cupsdSocketSource = resolveSpecialVariable(cupsdSocketSource, slot.Snap())

	emit("  # Mount cupsd socket from cups snap to client snap\n")
	emit("  mount options=(bind rw) %s -> /var/cups/cups.sock,\n", cupsdSocketSource)
	emit("  umount /var/cups/cups.sock,\n")

	apparmor.GenWritableProfile(emit, cupsdSocketSource, 1)
	apparmor.GenWritableProfile(emit, "/var/cups/cups.sock", 1)

	// TODO: figure out why this bit is necessary - without this, apparmor
	// denies trying to connect to the unix socket at /var/run/cups.sock with:
	//
	// apparmor="DENIED" operation="connect"
	// profile="snap.test-snapd-cups-consumer.bin"
	// name="/var/snap/test-snapd-cups-provider/common/cups.sock"
	// pid=3195747 comm="nc" requested_mask="wr" denied_mask="wr" fsuid=0 ouid=0

	spec.AddSnippet(fmt.Sprintf("%s rw,", cupsdSocketSource))

	return nil
}

func (iface *cupsInterface) MountConnectedPlug(spec *mount.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	// get the source directory for the bind mount
	var cupsdSocketSource string
	err := slot.Attr("cups-socket", &cupsdSocketSource)
	if err != nil {
		// should be impossible since we should fail this in the sanitize stage
		return fmt.Errorf("broken interface: %v", err)
	}

	cupsdSocketSource = resolveSpecialVariable(cupsdSocketSource, slot.Snap())

	return spec.AddMountEntry(osutil.MountEntry{
		Name: cupsdSocketSource,
		Dir:  "/var/cups/cups.sock",
		// the cups.sock file we are mounting over is very likely to exist
		// already and it is also very likely to be a socket file, so we need to
		// inform snap-update-ns that it is okay to perform our mount on top of
		// the existing file only even if the existing file is a socket file
		Options: []string{"bind", "rw", osutil.XSnapdKindFile(), osutil.XSnapdAllowSocketFile()},
	})
}

func init() {
	registerIface(&cupsInterface{
		commonInterface: commonInterface{
			name:                 "cups",
			summary:              cupsSummary,
			implicitOnCore:       false,
			implicitOnClassic:    false,
			baseDeclarationSlots: cupsBaseDeclarationSlots,
		},
	})
}
