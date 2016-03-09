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

// Package ifacestate implements the manager and state aspects
// responsible for the maintenance of interfaces the system.
package ifacestate

import (
	"github.com/ubuntu-core/snappy/interfaces"
	"github.com/ubuntu-core/snappy/interfaces/builtin"
	"github.com/ubuntu-core/snappy/overlord/state"
)

// InterfaceManager is responsible for the maintenance of interfaces in
// the system state.  It maintains interface connections, and also observes
// installed snaps to track the current set of available plugs and slots.
type InterfaceManager struct {
	// state holds a reference to persistent system state. Interface engine
	// uses the state to store list of "intents-to-connect" that were made by
	// the operator. Those intents are made effective during Ensure().
	state *state.State
	// repo contains all the interfaces, plugs and slots declared by snaps (or
	// added at runtime via the snap experimental command) and all the
	// connection made between plugs and slots.
	//
	// Repository is where the volatile runtime state is held. When a snaps are
	// installed, removed or changed in any way (updated) the repository must
	// be kept up-to-date as to what slots and plugs exist in the system.
	repo *interfaces.Repository
	// uDevRulesChanged is a flag monitoring if udev rules have been modified
	// on disk and need to be re-loaded by udev (with an explicit request).
	// Udev rules are re-loaded once, after making all the changes to udev
	// rules.
	uDevRulesChanged bool
	// aaProfilesLoaded is a set of all the loaded apparmor profiles. Profiles
	// are kept in kernel memory and are not persistent across reboots. This
	// set is here to help us purge unused profiles at runtime.
	//
	// NOTE: there's a systemd job that loads all existing profiles from
	// snappy-specific var directory early on boot. Those are not reflected
	// here.
	aaProfilesLoaded map[string]bool
}

// Manager returns a new InterfaceManager.
func Manager() (*InterfaceManager, error) {
	repo := interfaces.NewRepository()
	for _, iface := range builtin.Interfaces() {
		if err := repo.AddInterface(iface); err != nil {
			return nil, err
		}
	}
	return &InterfaceManager{repo: repo}, nil
}

// Connect initiates a change connecting an interface.
func (m *InterfaceManager) Connect(plugSnap, plugName, slotSnap, slotName string) error {
	return nil
}

// Disconnect initiates a change disconnecting an interface.
func (m *InterfaceManager) Disconnect(plugSnap, plugName, slotSnap, slotName string) error {
	return nil
}

// Init implements StateManager.Init.
func (m *InterfaceManager) Init(s *state.State) error {
	m.state = s
	// TODO: purge the repository here (or re-create it maybe)
	return nil
}

// Ensure implements StateManager.Ensure.
func (m *InterfaceManager) Ensure() error {
	// TODO: ensure that all connections stored in the state exist in the repository.
	//       - use snap meta-data published into the state by the snap manager
	// TODO: ensure that all automatically-connected connections are in place.
	//       - we need a new flag in interfaces.Interface to tell us this is needed
	// TODO: ensure that all the security files on disk match what we want to have.
	// TODO: ensure that no other security files are present (e.g. leftovers are removed)
	//       - use SyncDir for each of the sets generated by m.repo
	// TODO: ensure that all apparmor profiles we need to use are loaded into the kernel.
	//       - use return value from SyncDir() to load/reload profiles
	//       - keep track of m.aaProfilesLoaded
	// TODO: ensure that unused apparmor profiles are discarded from the kernel.
	//       - use interfaces/apparmor APIs to enumerate and discard profiles
	// TODO: ensure that udev rules are re-loaded if necessary.
	//       - keep track of result of SyncDir() specific to udev to know that
	//       this is needed
	//
	// TODO-TODO: ensure that we fire notification hooks (this needs more research first).
	return nil
}

// Stop implements StateManager.Stop.
func (m *InterfaceManager) Stop() error {
	return nil
}
