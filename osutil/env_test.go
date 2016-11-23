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

package osutil_test

import (
	"os"

	"gopkg.in/check.v1"

	"github.com/snapcore/snapd/osutil"
)

type envSuite struct{}

var _ = check.Suite(&envSuite{})

func (s *envSuite) TestEnvBoolTrue(c *check.C) {
	key := "__XYZZY__"
	os.Unsetenv(key)

	for _, s := range []string{
		"1", "t", "TRUE", // etc
	} {
		os.Setenv(key, s)
		c.Assert(os.Getenv(key), check.Equals, s)
		c.Check(osutil.EnvBool(key), check.Equals, true, check.Commentf(s))
	}
}

func (s *envSuite) TestEnvBoolFalse(c *check.C) {
	key := "__XYZZY__"
	os.Unsetenv(key)
	c.Assert(osutil.EnvBool(key), check.Equals, false)

	for _, s := range []string{
		"", "0", "f", "FALSE", // etc
	} {
		os.Setenv(key, s)
		c.Assert(os.Getenv(key), check.Equals, s)
		c.Check(osutil.EnvBool(key), check.Equals, false, check.Commentf(s))
	}
}
