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

package snapstate_test

import (
	"sort"
	"testing"
	"time"

	. "gopkg.in/check.v1"

	"github.com/ubuntu-core/snappy/overlord/snapstate"
	"github.com/ubuntu-core/snappy/overlord/state"
	"github.com/ubuntu-core/snappy/progress"
	"github.com/ubuntu-core/snappy/snappy"
)

func TestSnapManager(t *testing.T) { TestingT(t) }

type fakeBackend struct{}

func (backend *fakeBackend) Checkpoint(data []byte) error {
	return nil
}

type snapmgrTestSuite struct {
	state   *state.State
	snapmgr *snapstate.SnapManager
}

var _ = Suite(&snapmgrTestSuite{})

func (s *snapmgrTestSuite) SetUpTest(c *C) {
	s.state = state.New(nil)

	s.snapmgr = &snapstate.SnapManager{}
	s.snapmgr.Init(s.state)
}

func (s *snapmgrTestSuite) TestInstallAddsTasks(c *C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("install", "installing foo")
	snapstate.Install(chg, "some-snap", "some-channel")

	c.Assert(s.state.Changes(), HasLen, 1)
	c.Assert(chg.Tasks(), HasLen, 1)
	c.Assert(chg.Tasks()[0].Kind(), Equals, "install-snap")
}

func (s *snapmgrTestSuite) TestRemveAddsTasks(c *C) {
	s.state.Lock()
	defer s.state.Unlock()

	chg := s.state.NewChange("remove", "removing foo")
	snapstate.Remove(chg, "foo")

	c.Assert(s.state.Changes(), HasLen, 1)
	c.Assert(chg.Tasks(), HasLen, 1)
	c.Assert(chg.Tasks()[0].Kind(), Equals, "remove-snap")
}

func (s *snapmgrTestSuite) TestInitInits(c *C) {
	st := state.New(nil)
	snapmgr := &snapstate.SnapManager{}
	snapmgr.Init(st)

	c.Assert(snapstate.SnapManagerState(snapmgr), Equals, st)
	runner := snapstate.SnapManagerRunner(snapmgr)
	c.Assert(runner, FitsTypeOf, &state.TaskRunner{})

	handlers := runner.Handlers()
	keys := make([]string, 0, len(handlers))
	for hname := range handlers {
		keys = append(keys, hname)
	}
	sort.Strings(keys)
	c.Assert(keys, DeepEquals, []string{"install-snap", "remove-snap"})
}

func (s *snapmgrTestSuite) TestInstallIntegration(c *C) {
	installName := ""
	installChannel := ""
	snapstate.SnappyInstall = func(name, channel string, flags snappy.InstallFlags, meter progress.Meter) (string, error) {
		installName = name
		installChannel = channel
		return "", nil
	}

	s.state.Lock()
	chg := s.state.NewChange("install", "install a snap")
	err := snapstate.Install(chg, "some-snap", "some-channel")
	s.state.Unlock()

	c.Assert(err, IsNil)
	s.snapmgr.Ensure()

	// FIXME: use TaskRunner.Wait()
	for installName == "" {
		// wait
		time.Sleep(1 * time.Millisecond)
	}

	c.Assert(installName, Equals, "some-snap")
	c.Assert(installChannel, Equals, "some-channel")
}

func (s *snapmgrTestSuite) TestRemoveIntegration(c *C) {
	removeName := ""
	snapstate.SnappyRemove = func(name string, flags snappy.RemoveFlags, meter progress.Meter) error {
		removeName = name
		return nil
	}

	s.state.Lock()
	chg := s.state.NewChange("remove", "remove a snap")
	err := snapstate.Remove(chg, "some-remove-snap")
	s.state.Unlock()

	c.Assert(err, IsNil)
	s.snapmgr.Ensure()

	// FIXME: use TaskRunner.Wait()
	for removeName == "" {
		// wait
		time.Sleep(1 * time.Millisecond)
	}

	c.Assert(removeName, Equals, "some-remove-snap")
}
