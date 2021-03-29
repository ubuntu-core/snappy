// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2021 Canonical Ltd
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

package snapstate

import (
	"fmt"
	"sort"

	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/mount"
	"github.com/snapcore/snapd/overlord/ifacestate/ifacerepo"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/snap"
)

var gateAutoRefreshHookName = "gate-auto-refresh"

func affectedByRefresh(st *state.State, updates []*snap.Info) ([]string, error) {
	all, err := All(st)
	if err != nil {
		return nil, err
	}

	byBase := make(map[string][]string)
	for name, snapSt := range all {
		if !snapSt.Active {
			delete(all, name)
			continue
		}
		inf, err := snapSt.CurrentInfo()
		if err != nil {
			return nil, err
		}
		// optimize: do not consider snaps that don't have gate-auto-refresh hook.
		if inf.Hooks[gateAutoRefreshHookName] == nil {
			delete(all, name)
			continue
		}

		base := inf.Base
		if inf.Base == "" {
			base = "core"
		}
		byBase[base] = append(byBase[base], inf.InstanceName())
	}

	affected := make(map[string]bool)

	for _, up := range updates {
		// add self
		if all[up.InstanceName()] != nil {
			affected[up.InstanceName()] = true
		}

		// snaps that can trigger reboot
		// XXX: gadget refresh doesn't always require reboot, refine this
		if up.Type() == snap.TypeKernel || up.Type() == snap.TypeGadget {
			for _, snapSt := range all {
				affected[snapSt.InstanceName()] = true
			}
			continue
		}
		if up.Type() == snap.TypeBase || up.SnapName() == "core" {
			// affected by refresh of this base snap
			for _, snapName := range byBase[up.SnapName()] {
				affected[snapName] = true
			}
			continue
		}

		// no point in further checks
		if len(affected) == len(all) {
			break
		}

		repo := ifacerepo.Get(st)

		// consider slots provided by refreshed snap.
		for _, slotInfo := range up.Slots {
			conns, err := repo.Connected(up.InstanceName(), slotInfo.Name)
			if err != nil {
				return nil, err
			}
			for _, cref := range conns {
				// affected only if it wasn't optimized out above
				if all[cref.PlugRef.Snap] != nil {
					affected[cref.PlugRef.Snap] = true
				}
			}
		}

		// no point in further checks
		if len(affected) == len(all) {
			break
		}

		// consider plugs of the refreshed snap only if mount backend is involved
		for _, plugInfo := range up.Plugs {
			iface := repo.Interface(plugInfo.Interface)
			if iface == nil {
				return nil, fmt.Errorf("internal error: unknown interface %s", plugInfo.Interface)
			}
			if !usesMountBackend(iface) {
				continue
			}
			conns, err := repo.Connected(up.InstanceName(), plugInfo.Name)
			if err != nil {
				return nil, err
			}
			for _, cref := range conns {
				// affected only if it wasn't optimized out above
				if all[cref.SlotRef.Snap] != nil {
					affected[cref.SlotRef.Snap] = true
				}
			}
		}
	}

	aff := make([]string, len(affected))
	i := 0
	for snapName := range affected {
		aff[i] = snapName
		i++
	}
	sort.Strings(aff)
	return aff, nil
}

func usesMountBackend(iface interfaces.Interface) bool {
	type definer1 interface {
		MountConnectedSlot(*mount.Specification, *interfaces.ConnectedPlug, *interfaces.ConnectedSlot) error
	}
	type definer2 interface {
		MountConnectedPlug(*mount.Specification, *interfaces.ConnectedPlug, *interfaces.ConnectedSlot) error
	}
	type definer3 interface {
		MountPermanentPlug(*mount.Specification, *snap.PlugInfo) error
	}
	type definer4 interface {
		MountPermanentSlot(*mount.Specification, *snap.SlotInfo) error
	}

	if _, ok := iface.(definer1); ok {
		return true
	}
	if _, ok := iface.(definer2); ok {
		return true
	}
	if _, ok := iface.(definer3); ok {
		return true
	}
	if _, ok := iface.(definer4); ok {
		return true
	}
	return false
}