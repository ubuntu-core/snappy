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

package builtin_test

import (
	"fmt"
	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/interfaces"
	"github.com/snapcore/snapd/interfaces/apparmor"
	"github.com/snapcore/snapd/interfaces/builtin"
	"github.com/snapcore/snapd/interfaces/udev"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/testutil"
)

type acrnInterfaceSuite struct {
	testutil.BaseTest

	iface    interfaces.Interface
	slotInfo *snap.SlotInfo
	slot     *interfaces.ConnectedSlot
	plugInfo *snap.PlugInfo
	plug     *interfaces.ConnectedPlug
}

var _ = Suite(&acrnInterfaceSuite{
	iface: builtin.MustInterface("acrn"),
})

const acrnConsumerYaml = `name: consumer
version: 0
apps:
 app:
  plugs: [acrn]
`

const acrnGadgetYaml = `name: gadget
version: 0
type: gadget
slots:
  acrn:
`

func (s *acrnInterfaceSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)

	s.plug, s.plugInfo = MockConnectedPlug(c, acrnConsumerYaml, nil, "acrn")
	s.slot, s.slotInfo = MockConnectedSlot(c, acrnGadgetYaml, nil, "acrn")
}

func (s *acrnInterfaceSuite) TestName(c *C) {
	c.Assert(s.iface.Name(), Equals, "acrn")
}

func (s *acrnInterfaceSuite) TestSanitizeSlot(c *C) {
	c.Assert(interfaces.BeforePrepareSlot(s.iface, s.slotInfo), IsNil)
}

func (s *acrnInterfaceSuite) TestSanitizePlug(c *C) {
	c.Assert(interfaces.BeforePreparePlug(s.iface, s.plugInfo), IsNil)
}

func (s *acrnInterfaceSuite) TestAppArmorSpec(c *C) {
	spec := &apparmor.Specification{}
	c.Assert(spec.AddConnectedPlug(s.iface, s.plug, s.slot), IsNil)
	c.Assert(spec.SecurityTags(), DeepEquals, []string{"snap.consumer.app"})
	c.Assert(spec.SnippetForTag("snap.consumer.app"), testutil.Contains, "/sys/devices/system/cpu/cpu[0-9]*/online w,")
	c.Assert(spec.SnippetForTag("snap.consumer.app"), testutil.Contains, "/dev/acrn_vhm rw,")
	c.Assert(spec.SnippetForTag("snap.consumer.app"), testutil.Contains, "/dev/acrn_hsm rw,")
}

func (s *acrnInterfaceSuite) TestUDevSpec(c *C) {
	spec := &udev.Specification{}
	c.Assert(spec.AddConnectedPlug(s.iface, s.plug, s.slot), IsNil)
	c.Assert(spec.Snippets(), HasLen, 3)
	c.Assert(spec.Snippets(), testutil.Contains, `# acrn
SUBSYSTEM=="vhm", TAG+="snap_consumer_app"`)
	c.Assert(spec.Snippets(), testutil.Contains, fmt.Sprintf(`TAG=="snap_consumer_app", RUN+="%v/snap-device-helper $env{ACTION} snap_consumer_app $devpath $major:$minor"`, dirs.DistroLibExecDir))
}

func (s *acrnInterfaceSuite) TestStaticInfo(c *C) {
	si := interfaces.StaticInfoOf(s.iface)
	c.Assert(si.ImplicitOnCore, Equals, true)
	c.Assert(si.ImplicitOnClassic, Equals, true)
	c.Assert(si.Summary, Equals, `allows access to the ACRN device`)
	c.Assert(si.BaseDeclarationSlots, testutil.Contains, "acrn")
}

func (s *acrnInterfaceSuite) TestAutoConnect(c *C) {
	c.Assert(s.iface.AutoConnect(s.plugInfo, s.slotInfo), Equals, true)
}

func (s *acrnInterfaceSuite) TestInterfaces(c *C) {
	c.Check(builtin.Interfaces(), testutil.DeepContains, s.iface)
}
