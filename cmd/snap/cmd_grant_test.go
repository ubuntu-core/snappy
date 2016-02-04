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

package main_test

import (
	. "gopkg.in/check.v1"
)

func (s *SnapSuite) TestGrantHelp(c *C) {
	err := s.Execute([]string{"snap", "grant", "--help"})
	msg := `Usage:
  snap.test [OPTIONS] grant <snap>:<skill> <snap>:<skill slot>

$ snap grant <snap>:<skill> <snap>:<skill slot>

Grants the specific skill to the specific skill slot.

$ snap grant <snap>:<skill> <snap>

Grants the specific skill to the only skill slot in the provided snap that
matches the granted skill type. If more than one potential slot exists, the
command fails.

$ snap grant <skill> <snap>[:<skill slot>]

Without a name for the snap offering the skill, the skill name is looked at in
the gadget snap, the kernel snap, and then the os snap, in that order. The
first of these snaps that has a matching skill name is used and the command
proceeds as above.

Help Options:
  -h, --help                     Show this help message
`
	c.Assert(err.Error(), Equals, msg)
}

func (s *SnapSuite) TestGrantExplicitEverything(c *C) {
	client := NewLowLevelTestClient()
	s.UseTestClient(client)
	err := s.Execute([]string{
		"snap", "grant", "producer:skill", "consumer:slot"})
	c.Assert(err, IsNil)
	c.Assert(client.Request.Method, Equals, "POST")
	c.Assert(client.Request.URL.Path, Equals, "/2.0/skills")
	c.Assert(client.DecodedRequestBody(c), DeepEquals, map[string]interface{}{
		"action": "grant",
		"skill": map[string]interface{}{
			"snap": "producer",
			"name": "skill",
		},
		"slot": map[string]interface{}{
			"snap": "consumer",
			"name": "slot",
		},
	})
}

func (s *SnapSuite) TestGrantExplicitSkillImplicitSlot(c *C) {
	client := NewLowLevelTestClient()
	s.UseTestClient(client)
	err := s.Execute([]string{
		"snap", "grant", "producer:skill", "consumer"})
	c.Assert(err, IsNil)
	c.Assert(client.Request.Method, Equals, "POST")
	c.Assert(client.Request.URL.Path, Equals, "/2.0/skills")
	c.Assert(client.DecodedRequestBody(c), DeepEquals, map[string]interface{}{
		"action": "grant",
		"skill": map[string]interface{}{
			"snap": "producer",
			"name": "skill",
		},
		"slot": map[string]interface{}{
			"snap": "consumer",
			"name": "",
		},
	})
}

func (s *SnapSuite) TestGrantImplicitSkillExplicitSlot(c *C) {
	client := NewLowLevelTestClient()
	s.UseTestClient(client)
	err := s.Execute([]string{
		"snap", "grant", "skill", "consumer:slot"})
	c.Assert(err, IsNil)
	c.Assert(client.Request.Method, Equals, "POST")
	c.Assert(client.Request.URL.Path, Equals, "/2.0/skills")
	c.Assert(client.DecodedRequestBody(c), DeepEquals, map[string]interface{}{
		"action": "grant",
		"skill": map[string]interface{}{
			"snap": "",
			"name": "skill",
		},
		"slot": map[string]interface{}{
			"snap": "consumer",
			"name": "slot",
		},
	})
}

func (s *SnapSuite) TestGrantImplicitSkillImplicitSlot(c *C) {
	client := NewLowLevelTestClient()
	s.UseTestClient(client)
	err := s.Execute([]string{
		"snap", "grant", "skill", "consumer"})
	c.Assert(err, IsNil)
	c.Assert(client.Request.Method, Equals, "POST")
	c.Assert(client.Request.URL.Path, Equals, "/2.0/skills")
	c.Assert(client.DecodedRequestBody(c), DeepEquals, map[string]interface{}{
		"action": "grant",
		"skill": map[string]interface{}{
			"snap": "",
			"name": "skill",
		},
		"slot": map[string]interface{}{
			"snap": "consumer",
			"name": "",
		},
	})
}
