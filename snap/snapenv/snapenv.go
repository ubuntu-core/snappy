// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2015 Canonical Ltd
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

package snapenv

import (
	"os"
	"os/user"
	"path/filepath"

	"github.com/snapcore/snapd/arch"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/features"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/sys"
	"github.com/snapcore/snapd/snap"
)

// ExecEnv returns the full environment that is required for
// snap-{confine,exec}(like SNAP_{NAME,REVISION} etc are all set).
//
// It merges it with the existing os.Environ() and ensures the SNAP_*
// overrides the any pre-existing environment variables. For a classic
// snap, environment variables that are usually stripped out by ld.so
// when starting a setuid process are renamed by prepending
// PreservedUnsafePrefix -- which snap-exec will remove, restoring the
// variables to their original names.
//
// With the extra parameter additional environment variables can be
// supplied which will be set in the execution environment.
func ExecEnv(info *snap.Info) (osutil.Environment, error) {
	// Start with OS environment.
	env, err := osutil.OSEnvironment()
	if err != nil {
		// If environment is maliciously corrupted it may not parse correctly.
		return nil, err
	}

	// For snaps using classic confinement preserve variables that are
	// automatically discarded by executing setuid executables.
	if info.NeedsClassic() {
		env.EscapeUnsafeVariables()
	}

	// Set various SNAP_ environment variables as well as some non-SNAP variables,
	// depending on snap confinement mode. Note that this does not include environment
	// set by snap-exec.
	for k, v := range snapEnv(info) {
		env[k] = v
	}
	return env, nil
}

func snapEnv(info *snap.Info) osutil.Environment {
	// Environment variables with basic properties of a snap.
	env := basicEnv(info)
	if usr, err := user.Current(); err == nil && usr.HomeDir != "" {
		// Environment variables with values specific to the calling user.
		for k, v := range userEnv(info, usr.HomeDir) {
			env[k] = v
		}
	}
	return env
}

// basicEnv returns the app-level environment variables for a snap.
// Despite this being a bit snap-specific, this is in helpers.go because it's
// used by so many other modules, we run into circular dependencies if it's
// somewhere more reasonable like the snappy module.
func basicEnv(info *snap.Info) osutil.Environment {
	return osutil.Environment{
		// This uses CoreSnapMountDir because the computed environment
		// variables are conveyed to the started application process which
		// shall *either* execute with the new mount namespace where snaps are
		// always mounted on /snap OR it is a classically confined snap where
		// /snap is a part of the distribution package.
		//
		// For parallel-installs the mount namespace setup is making the
		// environment of each snap instance appear as if it's the only
		// snap, i.e. SNAP paths point to the same locations within the
		// mount namespace
		"SNAP":               filepath.Join(dirs.CoreSnapMountDir, info.SnapName(), info.Revision.String()),
		"SNAP_COMMON":        snap.CommonDataDir(info.SnapName()),
		"SNAP_DATA":          snap.DataDir(info.SnapName(), info.Revision),
		"SNAP_NAME":          info.SnapName(),
		"SNAP_INSTANCE_NAME": info.InstanceName(),
		"SNAP_INSTANCE_KEY":  info.InstanceKey,
		"SNAP_VERSION":       info.Version,
		"SNAP_REVISION":      info.Revision.String(),
		"SNAP_ARCH":          arch.DpkgArchitecture(),
		// see https://github.com/snapcore/snapd/pull/2732#pullrequestreview-18827193
		"SNAP_LIBRARY_PATH": "/var/lib/snapd/lib/gl:/var/lib/snapd/lib/gl32:/var/lib/snapd/void",
		"SNAP_REEXEC":       os.Getenv("SNAP_REEXEC"),
	}
}

// userEnv returns the user-level environment variables for a snap.
// Despite this being a bit snap-specific, this is in helpers.go because it's
// used by so many other modules, we run into circular dependencies if it's
// somewhere more reasonable like the snappy module.
func userEnv(info *snap.Info, home string) osutil.Environment {
	// To keep things simple the user variables always point to the
	// instance-specific directories.
	env := osutil.Environment{
		"SNAP_USER_COMMON": info.UserCommonDataDir(home),
		"SNAP_USER_DATA":   info.UserDataDir(home),
	}
	if info.NeedsClassic() {
		// Snaps using classic confinement don't have an override for
		// HOME but may have an override for XDG_RUNTIME_DIR.
		if !features.ClassicPreservesXdgRuntimeDir.IsEnabled() {
			env["XDG_RUNTIME_DIR"] = info.UserXdgRuntimeDir(sys.Geteuid())
		}
	} else {
		// Snaps using strict or devmode confinement get an override for both
		// HOME and XDG_RUNTIME_DIR.
		env["HOME"] = info.UserDataDir(home)
		env["XDG_RUNTIME_DIR"] = info.UserXdgRuntimeDir(sys.Geteuid())
	}
	return env
}
