// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2015 Canonical Ltd
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

package caps

import (
	"fmt"

	. "gopkg.in/check.v1"

	"github.com/ubuntu-core/snappy/testutil"
)

// BoolFileType

type BoolFileTypeSuite struct {
	testutil.BaseTest
	t Type
}

var _ = Suite(&BoolFileTypeSuite{
	t: &BoolFileType{},
})

func (s *BoolFileTypeSuite) TestName(c *C) {
	c.Assert(s.t.Name(), Equals, "bool-file")
}

func (s *BoolFileTypeSuite) TestSanitizeOK(c *C) {
	cap := &Capability{
		TypeName: "bool-file",
		Attrs:    map[string]string{"path": "path"},
	}
	err := s.t.Sanitize(cap)
	c.Assert(err, IsNil)
}

func (s *BoolFileTypeSuite) TestSanitizeWrongType(c *C) {
	cap := &Capability{
		TypeName: "other-type",
	}
	err := s.t.Sanitize(cap)
	c.Assert(err, ErrorMatches, "capability is not of type \"bool-file\"")
}

func (s *BoolFileTypeSuite) TestSanitizeMissingPath(c *C) {
	cap := &Capability{
		TypeName: "bool-file",
	}
	err := s.t.Sanitize(cap)
	c.Assert(err, ErrorMatches, "bool-file must contain the path attribute")
}

func (s *BoolFileTypeSuite) TestSecuritySnippet(c *C) {
	MockEvalSymlinks(&s.BaseTest, func(path string) (string, error) {
		return "real-path", nil
	})
	cap := &Capability{
		TypeName: "bool-file",
		Attrs:    map[string]string{"path": "path"},
	}
	snippet, err := s.t.SecuritySnippet(cap, SecurityApparmor)
	c.Assert(err, IsNil)
	c.Assert(snippet, DeepEquals, []byte("real-path rwl,\n"))
	snippet, err = s.t.SecuritySnippet(cap, SecuritySeccomp)
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
	snippet, err = s.t.SecuritySnippet(cap, SecurityDBus)
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
	snippet, err = s.t.SecuritySnippet(cap, "foo")
	c.Assert(err, ErrorMatches, `unknown security system`)
	c.Assert(snippet, IsNil)
}

func (s *BoolFileTypeSuite) TestDereferencePathSuccess(c *C) {
	MockEvalSymlinks(&s.BaseTest, func(path string) (string, error) {
		return "real-path", nil
	})
	cap := &Capability{
		TypeName: "bool-file",
		Attrs:    map[string]string{"path": "symbolic-path"},
	}
	path, err := s.t.(*BoolFileType).dereferencedPath(cap)
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "real-path")
}

func (s *BoolFileTypeSuite) TestDereferencePathError(c *C) {
	MockEvalSymlinks(&s.BaseTest, func(path string) (string, error) {
		return "", fmt.Errorf("broken symbolic link")
	})
	cap := &Capability{
		TypeName: "bool-file",
		Attrs:    map[string]string{"path": "symbolic-path"},
	}
	path, err := s.t.(*BoolFileType).dereferencedPath(cap)
	c.Assert(err, ErrorMatches, "bool-file path is invalid: broken symbolic link")
	c.Assert(path, Equals, "")
}

// TestType

type TestTypeSuite struct {
	t Type
}

var _ = Suite(&TestTypeSuite{
	t: &TestType{TypeName: "mock"},
})

// TestType has a working Name() function
func (s *TestTypeSuite) TestName(c *C) {
	c.Assert(s.t.Name(), Equals, "mock")
}

// TestType doesn't do any sanitization by default
func (s *TestTypeSuite) TestSanitizeOK(c *C) {
	cap := &Capability{
		TypeName: "mock",
	}
	err := s.t.Sanitize(cap)
	c.Assert(err, IsNil)
}

// TestType has provisions to customize sanitization
func (s *TestTypeSuite) TestSanitizeError(c *C) {
	t := &TestType{
		TypeName: "mock",
		SanitizeCallback: func(cap *Capability) error {
			return fmt.Errorf("sanitize failed")
		},
	}
	cap := &Capability{
		TypeName: "mock",
	}
	err := t.Sanitize(cap)
	c.Assert(err, ErrorMatches, "sanitize failed")
}

// TestType sanitization still checks for type identity
func (s *TestTypeSuite) TestSanitizeWrongType(c *C) {
	cap := &Capability{
		TypeName: "other-type",
	}
	err := s.t.Sanitize(cap)
	c.Assert(err, ErrorMatches, "capability is not of type \"mock\"")
}

// TestType hands out empty security snippets
func (s *TestTypeSuite) TestSecuritySnippet(c *C) {
	cap := &Capability{
		TypeName: "mock",
	}
	snippet, err := s.t.SecuritySnippet(cap, SecurityApparmor)
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
	snippet, err = s.t.SecuritySnippet(cap, SecuritySeccomp)
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
	snippet, err = s.t.SecuritySnippet(cap, SecurityDBus)
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
	snippet, err = s.t.SecuritySnippet(cap, "foo")
	c.Assert(err, IsNil)
	c.Assert(snippet, IsNil)
}
