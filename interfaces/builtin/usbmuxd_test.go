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

package builtin_test

import (
	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/apparmor"
	"github.com/snapcore/snapd/interfaces/builtin"
	"github.com/snapcore/snapd/interfaces/seccomp"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snaptest"
	"github.com/snapcore/snapd/testutil"
)

type UsbmuxdInterfaceSuite struct {
	iface    interfaces.Interface
	slotInfo *snap.SlotInfo
	slot     *interfaces.ConnectedSlot
	plugInfo *snap.PlugInfo
	plug     *interfaces.ConnectedPlug
}

const usbmuxdMockPlugSnapInfoYaml = `name: usbmuxd
version: 1.0
apps:
 app:
  command: foo
  plugs: [usbmuxd]
`

var _ = Suite(&UsbmuxdInterfaceSuite{
	iface: builtin.MustInterface("usbmuxd"),
})

func (s *UsbmuxdInterfaceSuite) SetUpTest(c *C) {
	s.slotInfo = &snap.SlotInfo{
		Snap: &snap.Info{
			SuggestedName: "usbmuxd",
		},
		Name:      "usbmuxd-daemon",
		Interface: "usbmuxd",
	}
	s.slot = interfaces.NewConnectedSlot(s.slotInfo, nil, nil)
	plugSnap := snaptest.MockInfo(c, usbmuxdMockPlugSnapInfoYaml, nil)
	s.plugInfo = plugSnap.Plugs["usbmuxd"]
	s.plug = interfaces.NewConnectedPlug(s.plugInfo, nil, nil)
}

func (s *UsbmuxdInterfaceSuite) TestName(c *C) {
	c.Assert(s.iface.Name(), Equals, "usbmuxd")
}

func (s *UsbmuxdInterfaceSuite) TestConnectedPlugSnippet(c *C) {
	apparmorSpec := &apparmor.Specification{}
	err := apparmorSpec.AddConnectedPlug(s.iface, s.plug, s.slot)
	c.Assert(err, IsNil)
	c.Assert(apparmorSpec.SecurityTags(), DeepEquals, []string{"snap.usbmuxd.app"})
	c.Assert(apparmorSpec.SnippetForTag("snap.usbmuxd.app"), testutil.Contains, `run/usbmuxd.sock`)

	seccompSpec := &seccomp.Specification{}
	err = seccompSpec.AddConnectedPlug(s.iface, s.plug, s.slot)
	c.Assert(err, IsNil)
	c.Assert(seccompSpec.SecurityTags(), DeepEquals, []string{"snap.usbmuxd.app"})
	c.Check(seccompSpec.SnippetForTag("snap.usbmuxd.app"), testutil.Contains, "bind\n")
}

func (s *UsbmuxdInterfaceSuite) TestSanitizeSlot(c *C) {
	c.Assert(interfaces.BeforePrepareSlot(s.iface, s.slotInfo), IsNil)
}

func (s *UsbmuxdInterfaceSuite) TestSanitizePlug(c *C) {
	c.Assert(interfaces.BeforePreparePlug(s.iface, s.plugInfo), IsNil)
}

func (s *UsbmuxdInterfaceSuite) TestInterfaces(c *C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}
