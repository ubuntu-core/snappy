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

// Package udev implements integration between snappy, udev and
// ubuntu-core-laucher around tagging character and block devices so that they
// can be accessed by applications.
//
// TODO: Document this better
package udev

import (
	"bytes"
	"fmt"

	"github.com/ubuntu-core/snappy/dirs"
	"github.com/ubuntu-core/snappy/interfaces"
	"github.com/ubuntu-core/snappy/osutil"
	"github.com/ubuntu-core/snappy/snap"
)

// Backend is responsible for maintaining udev rules.
type Backend struct{}

// Setup creates udev rules specific to a given snap.
// If any of the rules are changed or removed then udev database is reloaded.
//
// Since udev has no concept of a complain mode, developerMode is ignored.
//
// If the method fails it should be re-tried (with a sensible strategy) by the caller.
func (b *Backend) Setup(snapInfo *snap.Info, developerMode bool, repo *interfaces.Repository) error {
	snippets, err := repo.SecuritySnippetsForSnap(snapInfo.Name(), interfaces.SecurityUDev)
	if err != nil {
		return fmt.Errorf("cannot obtain udev security snippets for snap %q: %s", snapInfo.Name(), err)
	}
	content, err := b.combineSnippets(snapInfo, snippets)
	if err != nil {
		return fmt.Errorf("cannot obtain expected udev rules for snap %q: %s", snapInfo.Name(), err)
	}
	glob := fmt.Sprintf("70-%s.rules", interfaces.SecurityTagGlob(snapInfo))
	return ensureDirState(dirs.SnapUdevRulesDir, glob, content, snapInfo)
}

// Remove removes udev rules specific to a given snap.
// If any of the rules are removed then udev database is reloaded.
//
// This method should be called after removing a snap.
//
// If the method fails it should be re-tried (with a sensible strategy) by the caller.
func (b *Backend) Remove(snapInfo *snap.Info) error {
	glob := fmt.Sprintf("70-%s.rules", interfaces.SecurityTagGlob(snapInfo))
	return ensureDirState(dirs.SnapUdevRulesDir, glob, nil, snapInfo)
}

func ensureDirState(dir, glob string, content map[string]*osutil.FileState, snapInfo *snap.Info) error {
	var errReload error
	changed, removed, errEnsure := osutil.EnsureDirState(dir, glob, content)
	if len(changed) > 0 || len(removed) > 0 {
		// Try reload the rules regardless of errEnsure.
		errReload = ReloadRules()
	}
	if errEnsure != nil {
		return fmt.Errorf("cannot synchronize udev rules for snap %q: %s", snapInfo.Name(), errEnsure)
	}
	return errReload
}

// combineSnippets combines security snippets collected from all the interfaces
// affecting a given snap into a content map applicable to EnsureDirState.
func (b *Backend) combineSnippets(snapInfo *snap.Info, snippets map[string][][]byte) (content map[string]*osutil.FileState, err error) {
	for _, appInfo := range snapInfo.Apps {
		appSnippets := snippets[appInfo.Name]
		if len(appSnippets) == 0 {
			continue
		}
		var buf bytes.Buffer
		buf.WriteString("# This file is automatically generated.\n")
		for _, snippet := range appSnippets {
			buf.Write(snippet)
			buf.WriteRune('\n')
		}
		if content == nil {
			content = make(map[string]*osutil.FileState)
		}
		fname := fmt.Sprintf("70-%s.rules", interfaces.SecurityTag(appInfo))
		content[fname] = &osutil.FileState{Content: buf.Bytes(), Mode: 0644}
	}
	return content, nil
}
