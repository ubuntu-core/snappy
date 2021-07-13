/*
 * Copyright 2021 Intel Corporation, Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package builtin

import (
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/kmod"
)

const acrnSummary = `allows access to the ACRN device`

const acrnBaseDeclarationSlots = `
  acrn:
    allow-installation:
      slot-snap-type:
        - core
    deny-auto-connection: true
`

const acrnConnectedPlugAppArmor = `
# Description: Allow access to resources required by ACRN.
#   allow offline CPU cores
/sys/devices/system/cpu/cpu[0-9]*/online w,

#   allow write access to ACRN Virtio and Hypervisor service Module
/dev/acrn_vhm rw,
/dev/acrn_hsm rw,
`

var acrnConnectedPlugUDev = []string{
	`SUBSYSTEM=="vhm"`,
	`SUBSYSTEM=="hsm"`,
}

type acrnInterface struct {
	commonInterface
}

func (iface *acrnInterface) KModConnectedPlug(spec *kmod.Specification, plug *interfaces.ConnectedPlug, slot *interfaces.ConnectedSlot) error {
	/* ACRN Hypervisor Service Module (HSM) is supported since kernel 5.12 */
	_ = spec.AddModule("acrn")
	return nil
}

func init() {
	registerIface(&acrnInterface{commonInterface{
		name:                  "acrn",
		summary:               acrnSummary,
		implicitOnCore:        true,
		implicitOnClassic:     true,
		baseDeclarationSlots:  acrnBaseDeclarationSlots,
		connectedPlugAppArmor: acrnConnectedPlugAppArmor,
		connectedPlugUDev:     acrnConnectedPlugUDev,
	}})
}
