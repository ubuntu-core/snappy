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

package cgroup_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/godbus/dbus"
	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dbusutil"
	"github.com/snapcore/snapd/dbusutil/dbustest"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/features"
	"github.com/snapcore/snapd/sandbox/cgroup"
)

func enableFeatures(c *C, ff ...features.SnapdFeature) {
	c.Assert(os.MkdirAll(dirs.FeaturesDir, 0755), IsNil)
	for _, f := range ff {
		c.Assert(ioutil.WriteFile(f.ControlFile(), nil, 0755), IsNil)
	}
}

type trackingSuite struct{}

var _ = Suite(&trackingSuite{})

func (s *trackingSuite) SetUpTest(c *C) {
	dirs.SetRootDir(c.MkDir())
}

func (s *trackingSuite) TearDownTest(c *C) {
	dirs.SetRootDir("")
}

// CreateTransientScope is a no-op when refresh app awareness is off
func (s *trackingSuite) TestCreateTransientScopeFeatureDisabled(c *C) {
	noDBus := func() (*dbus.Conn, error) {
		return nil, fmt.Errorf("dbus should not have been used")
	}
	restore := dbusutil.MockConnections(noDBus, noDBus)
	defer restore()

	c.Assert(features.RefreshAppAwareness.IsEnabled(), Equals, false)
	err := cgroup.CreateTransientScope("snap.pkg.app")
	c.Check(err, IsNil)
}

// CreateTransientScope does stuff when refresh app awareness is on
func (s *trackingSuite) TestCreateTransientScopeFeatureEnabled(c *C) {
	// Pretend that refresh app awareness is enabled
	enableFeatures(c, features.RefreshAppAwareness)
	c.Assert(features.RefreshAppAwareness.IsEnabled(), Equals, true)
	// Pretend we are a non-root user so that session bus is used.
	restore := cgroup.MockOsGetuid(12345)
	defer restore()
	// Pretend our PID is this value.
	restore = cgroup.MockOsGetpid(312123)
	defer restore()
	// Rig the random UUID generator to return this value.
	uuid := "cc98cd01-6a25-46bd-b71b-82069b71b770"
	restore = cgroup.MockRandomUUID(uuid)
	defer restore()
	// Replace interactions with DBus so that only session bus is available and responds with our logic.
	conn, err := dbustest.Connection(func(msg *dbus.Message, n int) ([]*dbus.Message, error) {
		switch n {
		case 0:
			return []*dbus.Message{happyResponseToStartTransientUnit(c, msg, "snap.pkg.app."+uuid+".scope", 312123)}, nil
		}
		return nil, fmt.Errorf("unexpected message #%d: %s", n, msg)
	})
	c.Assert(err, IsNil)
	restore = dbusutil.MockSessionBus(conn)
	defer restore()
	// Replace the cgroup analyzer function
	restore = cgroup.MockCgroupProcessPathInTrackingCgroup(func(pid int) (string, error) {
		return "/user.slice/user-12345.slice/user@12345.service/snap.pkg.app." + uuid + ".scope", nil
	})
	defer restore()

	err = cgroup.CreateTransientScope("snap.pkg.app")
	c.Check(err, IsNil)
}

func happyResponseToStartTransientUnit(c *C, msg *dbus.Message, scopeName string, pid int) *dbus.Message {
	// XXX: Those types might live in a package somewhere
	type Property struct {
		Name  string
		Value interface{}
	}
	type Unit struct {
		Name  string
		Props []Property
	}
	// Signature of StartTransientUnit, string, string, array of Property and array of Unit (see above).
	requestSig := dbus.SignatureOf("", "", []Property{}, []Unit{})

	c.Assert(msg.Type, Equals, dbus.TypeMethodCall)
	c.Check(msg.Flags, Equals, dbus.Flags(0))
	c.Check(msg.Headers, DeepEquals, map[dbus.HeaderField]dbus.Variant{
		dbus.FieldDestination: dbus.MakeVariant("org.freedesktop.systemd1"),
		dbus.FieldPath:        dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/systemd1")),
		dbus.FieldInterface:   dbus.MakeVariant("org.freedesktop.systemd1.Manager"),
		dbus.FieldMember:      dbus.MakeVariant("StartTransientUnit"),
		dbus.FieldSignature:   dbus.MakeVariant(requestSig),
	})
	c.Check(msg.Body, DeepEquals, []interface{}{
		scopeName,
		"fail",
		[][]interface{}{
			{"PIDs", dbus.MakeVariant([]uint32{uint32(pid)})},
		},
		[][]interface{}{},
	})

	responseSig := dbus.SignatureOf(dbus.ObjectPath(""))
	return &dbus.Message{
		Type: dbus.TypeMethodReply,
		Headers: map[dbus.HeaderField]dbus.Variant{
			dbus.FieldReplySerial: dbus.MakeVariant(msg.Serial()),
			dbus.FieldSender:      dbus.MakeVariant(":1"), // This does not matter.
			// dbus.FieldDestination is provided automatically by DBus test helper.
			dbus.FieldSignature: dbus.MakeVariant(responseSig),
		},
		// The object path returned in the body is not used by snap run yet.
		Body: []interface{}{dbus.ObjectPath("/org/freedesktop/systemd1/job/1462")},
	}
}

func (s *trackingSuite) TestDoCreateTransientScopeHappy(c *C) {
	conn, err := dbustest.Connection(func(msg *dbus.Message, n int) ([]*dbus.Message, error) {
		switch n {
		case 0:
			return []*dbus.Message{happyResponseToStartTransientUnit(c, msg, "foo.scope", 312123)}, nil
		}
		return nil, fmt.Errorf("unexpected message #%d: %s", n, msg)
	})

	c.Assert(err, IsNil)
	defer conn.Close()
	err = cgroup.DoCreateTransientScope(conn, "foo.scope", 312123)
	c.Assert(err, IsNil)
}
