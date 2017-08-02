// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
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

const cameraSummary = `allows access to all cameras`

const cameraBaseDeclarationSlots = `
  camera:
    allow-installation:
      slot-snap-type:
        - core
    deny-auto-connection: true
`

const cameraConnectedPlugAppArmor = `
# Until we have proper device assignment, allow access to all cameras
/dev/video[0-9]* rw,

# Allow detection of cameras. Leaks plugged in USB device info
/sys/bus/usb/devices/ r,
/sys/devices/pci**/usb*/**/idVendor r,
/sys/devices/pci**/usb*/**/idProduct r,
/run/udev/data/c81:[0-9]* r, # video4linux (/dev/video*, etc)
/sys/class/video4linux/ r,
/sys/devices/pci**/usb*/**/video4linux/** r,
`

const cameraConnectedPlugUdev = `
# This file contains udev rules for camera devices.
#
# Do not edit this file, it will be overwritten on updates

KERNEL=="video[0-9]*", TAG+="%s"
`

func init() {
	registerIface(&commonInterface{
		name:                  "camera",
		summary:               cameraSummary,
		implicitOnCore:        true,
		implicitOnClassic:     true,
		baseDeclarationSlots:  cameraBaseDeclarationSlots,
		connectedPlugAppArmor: cameraConnectedPlugAppArmor,
		connectedPlugUdev:     cameraConnectedPlugUdev,
		reservedForOS:         true,
	})
}
