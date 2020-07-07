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

/*
 * Microstack is a full OpenStack in a single snap package.
 * Virtual machines are spawned as QEMU processes with libvirt acting as a management
 * daemon (including for activities such as applying AppArmor profiles).
 * Networking is provided largely via OpenVSwitch and Neutron with dnsmasq acting
 * as an auxiliary daemon. tun/tap kernel module is used for creating virtual interfaces.
 * Virtual machines rely on KVM for virtualization acceleration and on vhost
 * framework in the kernel (vhost_net, vhost_scsi, vhost_vsock).
 */

const microStackSupportSummary = `allows operating as the MicroStack service`

const microStackSupportBaseDeclarationPlugs = `
  microstack-support:
    allow-installation: false
    allow-auto-connection: true
`

const microStackSupportBaseDeclarationSlots = `
  microstack-support:
    allow-installation:
      slot-snap-type:
        - core
    allow-auto-connection: true
`

const microStackSupportConnectedPlugAppArmor = `

# Used by QEMU to work with the kernel-side virtio implementations.
/dev/vhost-net rw,
/dev/vhost-scsi rw,
/dev/vhost-vsock rw,
# Used by QEMU to work with VFIO (https://www.kernel.org/doc/Documentation/vfio.txt).
# For vfio hotplug on systems without static vfio (LP: #1775777)
# VFIO userspace driver interface.
/dev/vfio/vfio rw,
# Access to VFIO group character devices such as /dev/vfio/<group> where <group> is the group number.
/dev/vfio/* rw,
# Used by Nova for mounting images via qemu-nbd.
/dev/nbd* rw,

# Allow issuing ioctls to the Device Mapper for LVM tools.
/dev/mapper/control rw,
# Allow access to loop devices and loop-control to be able to associate a file with a loop device
# for the purpose of using a file-backed LVM setup.
/dev/loop-control rw,
/dev/loop[0-9]* rw,

# Description: this policy intentionally allows Microstack services to configure AppArmor
# as libvirt generates AppArmor profiles for the utility processes it spawns.
/sys/kernel/security/apparmor/{,**} r,
/sys/kernel/security/apparmor/.remove w,
/sys/kernel/security/apparmor/.replace w,

# Used by libvirt to work with IOMMU.
/sys/kernel/iommu_groups/ r,
/sys/kernel/iommu_groups/** r,
/sys/bus/pci/devices/**/iommu_group/** r,

# Used by libvirt's QEMU driver state initialization code path.
# The path used is hard-coded in libvirt to <huge-page-mnt-dir>/libvirt/qemu.
/dev/hugepages/libvirt/ rw,
/dev/hugepages/libvirt/** mrwklix,

# Used by libvirt to read information about available CPU and memory resources of a host.
# TODO(dmitriis): return this back in case libvirt fails.
# /sys/devices/system/cpu/ r,

# Used by QEMU to get the maximum number of memory regions allowed in the vhost kernel module.
/sys/module/vhost/parameters/max_mem_regions r,

# Used by libvirt (cgroup-related):
/sys/fs/cgroup/unified/cgroup.controllers r,

# TODO(dmitriis): Remove this.
# /sys/fs/cgroup/** rw,
# /sys/fs/cgroup/*/machine.slice/machine-qemu*/{,**} rw,

# Non-systemd layout: https://libvirt.org/cgroups.html#currentLayoutGeneric
/sys/fs/cgroup/*/ r,
/sys/fs/cgroup/*/machine/ rw,
/sys/fs/cgroup/*/machine/*/ rw,
/sys/fs/cgroup/*/machine/qemu-*.libvirt-qemu/** rw,

# systemd-layout: https://libvirt.org/cgroups.html#systemdLayout
/sys/fs/cgroup/*/machine.slice/machine-qemu*/{,**} rw,

@{PROC}/[0-9]*/cgroup r,
@{PROC}/cgroups r,

# Used by libvirt.
@{PROC}/cmdline r,
@{PROC}/filesystems r,
@{PROC}/mtrr w,
@{PROC}/@{pids}/environ r,
@{PROC}/@{pids}/sched r,

@{PROC}/*/status r,
# TODO: remove if not used.
# @{PROC}/*/ns/net r,

# TODO(dmitriis): Remove since it was moved to network-control with a modification.
# Used by libvirt to work with network devices created for VMs.
# E.g. getting operational state and speed of tap devices.
# /sys/class/net/tap*/* rw,

# TODO(dmitriis): Remove since the raw-usb interface covers that.
# For hostdev access. The actual devices will be added dynamically
# /sys/bus/usb/devices/ r,
# /sys/devices/**/usb[0-9]*/** r,
# libusb needs udev data about usb devices (~equal to content of lsusb -v)
# /run/udev/data/+usb* r,
# /run/udev/data/c16[6,7]* r,
# /run/udev/data/c18[0,8,9]* r,

# Libvirt needs access to the PCI config space in order to be able to reset devices.
/sys/devices/pci*/**/config rw,

# TODO(dmitriis) remove this
#required by libpmem init to fts_open()/fts_read() the symlinks in
# /sys/bus/nd/devices
# / r, # harmless on any lsb compliant system
/sys/bus/nd/devices/{,**/} r,
# # For ppc device-tree access by Libvirt.
# @{PROC}/device-tree/ r,
# @{PROC}/device-tree/** r,
# /sys/firmware/devicetree/** r,

# "virsh console" support
# /dev/pts/* rw,

# Used by libvirt.
/dev/ptmx rw,
# spice
owner /{dev,run}/shm/spice.* rw,

# Used by libvirt to create lock files for /dev/pts/<num> devices
# when handling virsh console access requests.
/run/lock/ r,
/run/lock/LCK.._pts_* rwk,

# Used by LVM tools.
/run/lock/lvm/** rwk,

# Allow running utility processes under the specialized AppArmor profiles.
# These profiles will prevent utility processes escaping confinement.
capability mac_admin,

# MicroStack services such as libvirt use has a server/client design where
# unix sockets are used for IPC.
capability chown,

# Required by Nova.
capability dac_override,
capability dac_read_search,
capability fowner,

# Used by libvirt to alter process capabilities via prctl.
capability setpcap,
# Used by libvirt to create device special files.
capability mknod,

# Allow libvirt to apply policy to spawned VM processes.
change_profile -> libvirt-[0-9a-f]*-[0-9a-f]*-[0-9a-f]*-[0-9a-f]*-[0-9a-f]*,

# Allow sending signals to the spawned VM processes.
signal (read, send) peer=libvirt-*,

# Allow reading certain proc entries, see ptrace(2) "Ptrace access mode checking".
# For ourselves.
ptrace (read, trace) peer=@{profile_name},
# For VM processes libvirt spawns.
ptrace (read, trace) peer=libvirt-*,

# Used by neutron-ovn-agent.
# /run/netns/ r,  # TODO(dmitriis) remove this
unmount /run/netns/ovnmeta-*,
`

const microStackSupportConnectedPlugSecComp = `
# Description: allow MicroStack to operate by allowing the necessary system calls to be used by various services.
# (libvirt, qemu, qemu-img, dnsmasq, Nova, Neutron, Keystone, Glance, Cinder)

# Note that this profile necessarily contains the union of all the syscalls each of the
# utilities requires. We rely on MicroStack to generate specific AppArmor profiles
# for each child process, to further restrict their abilities.
`
var microStackConnectedPlugUDev = []string{
	`KERNEL=="vhost-net"`,
	`KERNEL=="vhost-scsi"`,
	`KERNEL=="vhost-vsock"`,
	`SUBSYSTEM=="block", KERNEL=="nbd[0-9]*"`,
	`SUBSYSTEM=="misc", KERNEL=="vfio"`,
	`SUBSYSTEM=="vfio", KERNEL=="[0-9]*"`,
	`SUBSYSTEM=="block", KERNEL=="loop[0-9]*"`,
	`SUBSYSTEM=="misc", KERNEL=="loop-control"`,
	`SUBSYSTEM=="misc", KERNEL=="device-mapper"`,
}

type microStackInterface struct {
	commonInterface
}

var microStackSupportConnectedPlugKmod = []string{`vhost`, `vhost-net`, `vhost-scsi`, `vhost-vsock`, `pci-stub`, `vfio`, `nbd`, `dm-mod`}

func init() {
	registerIface(&microStackInterface{commonInterface{
		name:                  "microstack-support",
		summary:               microStackSupportSummary,
		implicitOnCore:        true,
		implicitOnClassic:     true,
		baseDeclarationSlots:  microStackSupportBaseDeclarationSlots,
		baseDeclarationPlugs:  microStackSupportBaseDeclarationPlugs,
		connectedPlugAppArmor: microStackSupportConnectedPlugAppArmor,
		connectedPlugSecComp:  microStackSupportConnectedPlugSecComp,
		connectedPlugUDev:     microStackConnectedPlugUDev,
		connectedPlugKModModules: microStackSupportConnectedPlugKmod,
	}})
}
