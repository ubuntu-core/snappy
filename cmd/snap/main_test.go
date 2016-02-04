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
	"testing"

	. "gopkg.in/check.v1"

	. "github.com/ubuntu-core/snappy/cmd/snap"
	"github.com/ubuntu-core/snappy/testutil"
)

// Hook up check.v1 into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

type SnapSuite struct {
	testutil.BaseTest
}

var _ = Suite(&SnapSuite{})

func (s *SnapSuite) UseTestClient(client Client) {
	origGetClient := GetClient
	s.BaseTest.AddCleanup(func() { GetClient = origGetClient })
	GetClient = func() Client { return client }
}

// Execute runs snappy as if invoked on command line
func (s *SnapSuite) Execute(args []string) error {
	_, err := Parser().ParseArgs(args)
	return err
}
