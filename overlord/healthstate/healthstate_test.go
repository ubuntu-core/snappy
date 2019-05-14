// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019 Canonical Ltd
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

package healthstate_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/check.v1"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/overlord"
	"github.com/snapcore/snapd/overlord/healthstate"
	"github.com/snapcore/snapd/overlord/hookstate"
	"github.com/snapcore/snapd/overlord/snapstate"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/snap/snaptest"
	"github.com/snapcore/snapd/store/storetest"
	"github.com/snapcore/snapd/testutil"
)

func TestHealthState(t *testing.T) { check.TestingT(t) }

type healthSuite struct {
	testutil.BaseTest
	o       *overlord.Overlord
	se      *overlord.StateEngine
	state   *state.State
	hookMgr *hookstate.HookManager
	info    *snap.Info
}

var _ = check.Suite(&healthSuite{})

func (s *healthSuite) SetUpTest(c *check.C) {
	s.BaseTest.SetUpTest(c)
	s.AddCleanup(healthstate.MockCheckTimeout(time.Second))
	dirs.SetRootDir(c.MkDir())

	s.o = overlord.Mock()
	s.state = s.o.State()

	var err error
	s.hookMgr, err = hookstate.Manager(s.state, s.o.TaskRunner())
	c.Assert(err, check.IsNil)
	s.se = s.o.StateEngine()
	s.o.AddManager(s.hookMgr)
	s.o.AddManager(s.o.TaskRunner())

	healthstate.Init(s.hookMgr)

	s.state.Lock()
	defer s.state.Unlock()

	snapstate.ReplaceStore(s.state, storetest.Store{})
	sideInfo := &snap.SideInfo{RealName: "test-snap", Revision: snap.R(42)}
	snapstate.Set(s.state, "test-snap", &snapstate.SnapState{
		Sequence: []*snap.SideInfo{sideInfo},
		Current:  snap.R(42),
		Active:   true,
		SnapType: "app",
	})
	s.info = snaptest.MockSnapCurrent(c, "{name: test-snap, version: v1}", sideInfo)
}

func (s *healthSuite) TearDownTest(c *check.C) {
	s.hookMgr.StopHooks()
	s.se.Stop()
	s.BaseTest.TearDownTest(c)
}

func (s *healthSuite) TestHealthNoHook(c *check.C) {
	s.testHealth(c, false, false)
}

func (s *healthSuite) TestHealthFailingHook(c *check.C) {
	s.testHealth(c, true, true)
}

func (s *healthSuite) TestHealth(c *check.C) {
	s.testHealth(c, true, false)
}

func (s *healthSuite) testHealth(c *check.C, withScript, failScript bool) {
	var cmd *testutil.MockCmd
	if failScript {
		cmd = testutil.MockCommand(c, "snap", "exit 1")
	} else {
		cmd = testutil.MockCommand(c, "snap", "exit 0")
	}
	if withScript {
		hookFn := filepath.Join(s.info.MountDir(), "meta", "hooks", "check-health")
		c.Assert(os.MkdirAll(filepath.Dir(hookFn), 0755), check.IsNil)
		// the hook won't actually be called, but needs to exist
		c.Assert(ioutil.WriteFile(hookFn, nil, 0755), check.IsNil)
	}

	s.state.Lock()
	task := healthstate.CheckHook(s.state, "test-snap", snap.R(42))
	change := s.state.NewChange("kind", "summary")
	change.AddTask(task)
	s.state.Unlock()

	c.Assert(task.Kind(), check.Equals, "run-hook")
	var hooksup hookstate.HookSetup

	s.state.Lock()
	err := task.Get("hook-setup", &hooksup)
	s.state.Unlock()
	c.Check(err, check.IsNil)

	c.Check(hooksup, check.DeepEquals, hookstate.HookSetup{
		Snap:        "test-snap",
		Hook:        "check-health",
		Revision:    snap.R(42),
		Optional:    true,
		Timeout:     time.Second,
		IgnoreError: false,
		TrackError:  false,
	})

	t0 := time.Now()
	s.se.Ensure()
	s.se.Wait()
	tf := time.Now()
	var healths map[string]healthstate.HealthState
	s.state.Lock()
	status := change.Status()
	err = s.state.Get("health", &healths)
	s.state.Unlock()

	if failScript {
		c.Assert(status, check.Equals, state.ErrorStatus)
	} else {
		c.Assert(status, check.Equals, state.DoneStatus)
	}
	if withScript {
		c.Assert(err, check.IsNil)
		c.Assert(healths, check.HasLen, 1)
		c.Assert(healths["test-snap"], check.NotNil)
		health := healths["test-snap"]
		c.Check(health.Revision, check.Equals, snap.R(42))
		c.Check(health.Status, check.Equals, healthstate.UnknownStatus)
		if failScript {
			c.Check(health.Message, check.Equals, "hook failed")
			c.Check(health.Code, check.Equals, "snapd-hook-failed")
		} else {
			c.Check(health.Message, check.Equals, "hook did not call set-health")
			c.Check(health.Code, check.Equals, "")
		}
		c.Check(health.Timestamp.After(t0) && health.Timestamp.Before(tf), check.Equals, true,
			check.Commentf("%s ⩼ %s ⩼ %s", t0.Format(time.StampNano), health.Timestamp.Format(time.StampNano), tf.Format(time.StampNano)))
		c.Check(cmd.Calls(), check.DeepEquals, [][]string{{"snap", "run", "--hook", "check-health", "-r", "42", "test-snap"}})
	} else {
		// no script -> no health
		c.Assert(err, check.Equals, state.ErrNoState)
		c.Check(healths, check.IsNil)
		c.Check(cmd.Calls(), check.HasLen, 0)
	}
}

func (*healthSuite) TestStatus(c *check.C) {
	for i, str := range healthstate.KnownStatuses {
		status, err := healthstate.StatusLookup(str)
		c.Check(err, check.IsNil, check.Commentf("%v", str))
		c.Check(status, check.Equals, healthstate.HealthStatus(i), check.Commentf("%v", str))
		c.Check(healthstate.HealthStatus(i).String(), check.Equals, str, check.Commentf("%v", str))
	}
	status, err := healthstate.StatusLookup("rabbits")
	c.Check(status, check.Equals, healthstate.HealthStatus(-1))
	c.Check(err, check.ErrorMatches, `invalid status "rabbits".*`)
	c.Check(status.String(), check.Equals, "invalid (-1)")
}
