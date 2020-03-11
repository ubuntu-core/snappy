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
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/arch"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/features"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/sys"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/testutil"
)

func Test(t *testing.T) { TestingT(t) }

type HTestSuite struct {
	testutil.BaseTest
}

var _ = Suite(&HTestSuite{})

var mockYaml = []byte(`name: snapname
version: 1.0
apps:
 app:
  command: run-app
hooks:
 configure:
`)

var mockSnapInfo = &snap.Info{
	SuggestedName: "foo",
	Version:       "1.0",
	SideInfo: snap.SideInfo{
		Revision: snap.R(17),
	},
}
var mockClassicSnapInfo = &snap.Info{
	SuggestedName: "foo",
	Version:       "1.0",
	SideInfo: snap.SideInfo{
		Revision: snap.R(17),
	},
	Confinement: snap.ClassicConfinement,
}

func (s *HTestSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.BaseTest.AddCleanup(snap.MockSanitizePlugsSlots(func(snapInfo *snap.Info) {}))
}

func (s *HTestSuite) TearDownTest(c *C) {
	s.BaseTest.TearDownTest(c)
}

func (ts *HTestSuite) TestBasic(c *C) {
	env := basicEnv(mockSnapInfo)
	c.Assert(env, DeepEquals, osutil.Environment{
		"SNAP":               fmt.Sprintf("%s/foo/17", dirs.CoreSnapMountDir),
		"SNAP_COMMON":        "/var/snap/foo/common",
		"SNAP_DATA":          "/var/snap/foo/17",
		"SNAP_NAME":          "foo",
		"SNAP_INSTANCE_NAME": "foo",
		"SNAP_INSTANCE_KEY":  "",
		"SNAP_VERSION":       "1.0",
		"SNAP_REVISION":      "17",
		"SNAP_ARCH":          arch.DpkgArchitecture(),
		"SNAP_LIBRARY_PATH":  "/var/lib/snapd/lib/gl:/var/lib/snapd/lib/gl32:/var/lib/snapd/void",
		"SNAP_REEXEC":        "",
	})
}

func (ts *HTestSuite) TestUser(c *C) {
	env := userEnv(mockSnapInfo, "/root")
	c.Assert(env, DeepEquals, osutil.Environment{
		"SNAP_USER_COMMON": "/root/snap/foo/common",
		"SNAP_USER_DATA":   "/root/snap/foo/17",
		"HOME":             "/root/snap/foo/17",
		"XDG_RUNTIME_DIR":  fmt.Sprintf("/run/user/%d/snap.foo", sys.Geteuid()),
	})
}

func (ts *HTestSuite) TestUserForClassicConfinement(c *C) {
	dirs.SetRootDir(c.MkDir())
	defer dirs.SetRootDir("/")
	c.Assert(os.MkdirAll(dirs.FeaturesDir, 0755), IsNil)

	// With the classic-preserves-xdg-runtime-dir feature disabled the snap
	// per-user environment contains an override for XDG_RUNTIME_DIR.
	env := userEnv(mockClassicSnapInfo, "/root")
	c.Assert(env, DeepEquals, osutil.Environment{
		// NOTE: Both HOME and XDG_RUNTIME_DIR are not defined here.
		"SNAP_USER_COMMON": "/root/snap/foo/common",
		"SNAP_USER_DATA":   "/root/snap/foo/17",
		"XDG_RUNTIME_DIR":  fmt.Sprintf(dirs.GlobalRootDir+"/run/user/%d/snap.foo", sys.Geteuid()),
	})

	// With the classic-preserves-xdg-runtime-dir feature enabled the snap
	// per-user environment contains no overrides for XDG_RUNTIME_DIR.
	f := features.ClassicPreservesXdgRuntimeDir
	c.Assert(ioutil.WriteFile(f.ControlFile(), nil, 0644), IsNil)
	env = userEnv(mockClassicSnapInfo, "/root")
	c.Assert(env, DeepEquals, osutil.Environment{
		// NOTE: Both HOME and XDG_RUNTIME_DIR are not defined here.
		"SNAP_USER_COMMON": "/root/snap/foo/common",
		"SNAP_USER_DATA":   "/root/snap/foo/17",
	})
}

func (s *HTestSuite) TestSnapRunSnapExecEnv(c *C) {
	info, err := snap.InfoFromSnapYaml(mockYaml)
	c.Assert(err, IsNil)
	info.SideInfo.Revision = snap.R(42)

	usr, err := user.Current()
	c.Assert(err, IsNil)

	homeEnv := os.Getenv("HOME")
	defer os.Setenv("HOME", homeEnv)

	for _, withHomeEnv := range []bool{true, false} {
		if !withHomeEnv {
			os.Setenv("HOME", "")
		}

		env := snapEnv(info)
		c.Assert(env, DeepEquals, osutil.Environment{
			"SNAP":               fmt.Sprintf("%s/snapname/42", dirs.CoreSnapMountDir),
			"SNAP_COMMON":        "/var/snap/snapname/common",
			"SNAP_DATA":          "/var/snap/snapname/42",
			"SNAP_NAME":          "snapname",
			"SNAP_INSTANCE_NAME": "snapname",
			"SNAP_INSTANCE_KEY":  "",
			"SNAP_VERSION":       "1.0",
			"SNAP_REVISION":      "42",
			"SNAP_ARCH":          arch.DpkgArchitecture(),
			"SNAP_LIBRARY_PATH":  "/var/lib/snapd/lib/gl:/var/lib/snapd/lib/gl32:/var/lib/snapd/void",
			"SNAP_REEXEC":        "",
			"SNAP_USER_COMMON":   fmt.Sprintf("%s/snap/snapname/common", usr.HomeDir),
			"SNAP_USER_DATA":     fmt.Sprintf("%s/snap/snapname/42", usr.HomeDir),
			"HOME":               fmt.Sprintf("%s/snap/snapname/42", usr.HomeDir),
			"XDG_RUNTIME_DIR":    fmt.Sprintf("/run/user/%d/snap.snapname", sys.Geteuid()),
		})
	}
}

func (s *HTestSuite) TestParallelInstallSnapRunSnapExecEnv(c *C) {
	info, err := snap.InfoFromSnapYaml(mockYaml)
	c.Assert(err, IsNil)
	info.SideInfo.Revision = snap.R(42)

	usr, err := user.Current()
	c.Assert(err, IsNil)

	homeEnv := os.Getenv("HOME")
	defer os.Setenv("HOME", homeEnv)

	// pretend it's snapname_foo
	info.InstanceKey = "foo"

	for _, withHomeEnv := range []bool{true, false} {
		if !withHomeEnv {
			os.Setenv("HOME", "")
		}

		env := snapEnv(info)
		c.Check(env, DeepEquals, osutil.Environment{
			// Those are mapped to snap-specific directories by
			// mount namespace setup
			"SNAP":               fmt.Sprintf("%s/snapname/42", dirs.CoreSnapMountDir),
			"SNAP_COMMON":        "/var/snap/snapname/common",
			"SNAP_DATA":          "/var/snap/snapname/42",
			"SNAP_NAME":          "snapname",
			"SNAP_INSTANCE_NAME": "snapname_foo",
			"SNAP_INSTANCE_KEY":  "foo",
			"SNAP_VERSION":       "1.0",
			"SNAP_REVISION":      "42",
			"SNAP_ARCH":          arch.DpkgArchitecture(),
			"SNAP_LIBRARY_PATH":  "/var/lib/snapd/lib/gl:/var/lib/snapd/lib/gl32:/var/lib/snapd/void",
			"SNAP_REEXEC":        "",
			// User's data directories are not mapped to
			// snap-specific ones
			"SNAP_USER_COMMON": fmt.Sprintf("%s/snap/snapname_foo/common", usr.HomeDir),
			"SNAP_USER_DATA":   fmt.Sprintf("%s/snap/snapname_foo/42", usr.HomeDir),
			"HOME":             fmt.Sprintf("%s/snap/snapname_foo/42", usr.HomeDir),
			"XDG_RUNTIME_DIR":  fmt.Sprintf("/run/user/%d/snap.snapname_foo", sys.Geteuid()),
		})
	}
}

func (ts *HTestSuite) TestParallelInstallUser(c *C) {
	info := *mockSnapInfo
	info.InstanceKey = "bar"
	env := userEnv(&info, "/root")

	c.Assert(env, DeepEquals, osutil.Environment{
		"SNAP_USER_COMMON": "/root/snap/foo_bar/common",
		"SNAP_USER_DATA":   "/root/snap/foo_bar/17",
		"HOME":             "/root/snap/foo_bar/17",
		"XDG_RUNTIME_DIR":  fmt.Sprintf("/run/user/%d/snap.foo_bar", sys.Geteuid()),
	})
}

func (ts *HTestSuite) TestParallelInstallUserForClassicConfinement(c *C) {
	dirs.SetRootDir(c.MkDir())
	defer dirs.SetRootDir("/")
	c.Assert(os.MkdirAll(dirs.FeaturesDir, 0755), IsNil)

	info := *mockClassicSnapInfo
	info.InstanceKey = "bar"

	// With the classic-preserves-xdg-runtime-dir feature disabled the snap
	// per-user environment contains an override for XDG_RUNTIME_DIR.
	env := userEnv(&info, "/root")
	c.Assert(env, DeepEquals, osutil.Environment{
		"SNAP_USER_COMMON": "/root/snap/foo_bar/common",
		"SNAP_USER_DATA":   "/root/snap/foo_bar/17",
		"XDG_RUNTIME_DIR":  fmt.Sprintf(dirs.GlobalRootDir+"/run/user/%d/snap.foo_bar", sys.Geteuid()),
	})

	// With the classic-preserves-xdg-runtime-dir feature enabled the snap
	// per-user environment contains no overrides for XDG_RUNTIME_DIR.
	f := features.ClassicPreservesXdgRuntimeDir
	c.Assert(ioutil.WriteFile(f.ControlFile(), nil, 0644), IsNil)
	env = userEnv(&info, "/root")
	c.Assert(env, DeepEquals, osutil.Environment{
		// NOTE, Both HOME and XDG_RUNTIME_DIR are not defined here.
		"SNAP_USER_COMMON": "/root/snap/foo_bar/common",
		"SNAP_USER_DATA":   "/root/snap/foo_bar/17",
	})
}

func setenvWithReset(s *HTestSuite, key string, val string) {
	tmpdirEnv, tmpdirFound := os.LookupEnv("TMPDIR")
	os.Setenv("TMPDIR", "/var/tmp")
	if tmpdirFound {
		s.AddCleanup(func() { os.Setenv("TMPDIR", tmpdirEnv) })
	} else {
		s.AddCleanup(func() { os.Unsetenv("TMPDIR") })
	}
}

func (s *HTestSuite) TestExecEnvNoRenameTMPDIRForNonClassic(c *C) {
	setenvWithReset(s, "TMPDIR", "/var/tmp")

	env, err := ExecEnv(mockSnapInfo)
	c.Assert(err, IsNil)
	c.Assert(env["TMPDIR"], Equals, "/var/tmp")
	c.Assert(env["SNAP_SAVED_TMPDIR"], Equals, "")
}

func (s *HTestSuite) TestExecEnvRenameTMPDIRForClassic(c *C) {
	setenvWithReset(s, "TMPDIR", "/var/tmp")

	env, err := ExecEnv(mockClassicSnapInfo)
	c.Assert(err, IsNil)
	c.Assert(env["TMPDIR"], Equals, "")
	c.Assert(env["SNAP_SAVED_TMPDIR"], Equals, "/var/tmp")
}
