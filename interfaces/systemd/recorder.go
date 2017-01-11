// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
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

package systemd

import (
	"fmt"
)

// Recorder assists in collecting mount entries associated with an interface.
//
// Unlike the Backend itself (which is stateless and non-persistent) this type
// holds internal state that is used by the mount backend during the interface
// setup process.
type Recorder struct {
	Services map[string]Service
}

// AddService adds a new systemd service unit.
func (rec *Recorder) AddService(name string, s Service) error {
	if old, ok := rec.Services[name]; ok && old != s {
		return fmt.Errorf("interface requires conflicting system needs")
	}
	if rec.Services == nil {
		rec.Services = make(map[string]Service)
	}
	rec.Services[name] = s
	return nil
}
