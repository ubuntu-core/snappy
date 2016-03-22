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

package state_test

import (
	"fmt"

	. "gopkg.in/check.v1"

	"github.com/ubuntu-core/snappy/overlord/state"
	"github.com/ubuntu-core/snappy/testutil"
)

type taskSuite struct{}

var _ = Suite(&taskSuite{})

func (ts *taskSuite) TestNewTask(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	c.Check(t.Kind(), Equals, "download")
	c.Check(t.Summary(), Equals, "1...")
}

func (ts *taskSuite) TestGetSet(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	t.Set("a", 1)

	var v int
	err := t.Get("a", &v)
	c.Assert(err, IsNil)
	c.Check(v, Equals, 1)
}

func (ts *taskSuite) TestStatusAndSetStatus(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	c.Check(t.Status(), Equals, state.RunningStatus)

	t.SetStatus(state.DoneStatus)

	c.Check(t.Status(), Equals, state.DoneStatus)
}

func (ts *taskSuite) TestProgressAndSetProgress(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	t.SetProgress(2, 99)

	cur, tot := t.Progress()

	c.Check(cur, Equals, 2)
	c.Check(tot, Equals, 99)
}

func (ts *taskSuite) TestProgressDefaults(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	c.Check(t.Status(), Equals, state.RunningStatus)
	cur, tot := t.Progress()
	c.Check(cur, Equals, 0)
	c.Check(tot, Equals, 1)

	t.SetStatus(state.WaitingStatus)
	cur, tot = t.Progress()
	c.Check(cur, Equals, 0)
	c.Check(tot, Equals, 1)

	t.SetStatus(state.RunningStatus)
	cur, tot = t.Progress()
	c.Check(cur, Equals, 0)
	c.Check(tot, Equals, 1)

	t.SetStatus(state.DoneStatus)
	cur, tot = t.Progress()
	c.Check(cur, Equals, 1)
	c.Check(tot, Equals, 1)

	t.SetStatus(state.ErrorStatus)
	cur, tot = t.Progress()
	c.Check(cur, Equals, 1)
	c.Check(tot, Equals, 1)
}

func (ts *taskSuite) TestState(c *C) {
	st := state.New(nil)
	st.Lock()
	t := st.NewTask("download", "1...")
	st.Unlock()

	c.Assert(t.State(), Equals, st)
}

func (ts *taskSuite) TestTaskMarshalsWaitFor(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	t2.WaitFor(t1)

	d, err := t2.MarshalJSON()
	c.Assert(err, IsNil)

	needle := fmt.Sprintf(`"wait-tasks":["%s"`, t1.ID())
	c.Assert(string(d), testutil.Contains, needle)
}

func (ts *taskSuite) TestTaskWaitFor(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	t2.WaitFor(t1)

	c.Assert(t2.WaitTasks(), DeepEquals, []*state.Task{t1})
	c.Assert(t2.Status(), Equals, state.WaitingStatus)

	c.Assert(t1.HaltTasks(), DeepEquals, []*state.Task{t2})
}

func (cs *taskSuite) TestLogf(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	for i := 0; i < 20; i++ {
		t.Logf("Message #%d", i)
	}

	log := t.Log()
	c.Assert(log, HasLen, 10)
	for i := 0; i < 10; i++ {
		c.Assert(log[i], Equals, fmt.Sprintf("INFO: Message #%d", i+10))
	}
}

func (cs *taskSuite) TestErrorf(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")

	t.Errorf("Some %s", "error")
	c.Assert(t.Log(), DeepEquals, []string{"ERROR: Some error"})
}

func (ts *taskSuite) TestTaskMarshalsLog(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t := st.NewTask("download", "1...")
	t.Logf("foo")

	d, err := t.MarshalJSON()
	c.Assert(err, IsNil)

	c.Assert(string(d), testutil.Contains, `"log":["INFO: foo"]`)
}

// TODO: Better testing of full task roundtripping via JSON.

func (ts *taskSuite) TestNeedsLock(c *C) {
	st := state.New(nil)
	st.Lock()
	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	st.Unlock()

	funcs := []func(){
		func() { t1.Status() },
		func() { t1.SetStatus(state.DoneStatus) },
		func() { t1.Set("a", 1) },
		func() { t1.Get("a", nil) },
		func() { t1.WaitTasks() },
		func() { t2.WaitFor(t1) },
		func() { t1.HaltTasks() },
		func() { t1.SetProgress(2, 2) },
		func() { t1.Progress() },
		func() { t1.Log() },
		func() { t1.Logf("") },
		func() { t1.Errorf("") },
	}

	for i, f := range funcs {
		c.Logf("Testing function #%d", i)
		c.Assert(f, PanicMatches, "internal error: accessing state without lock")
	}
}

func (cs *taskSuite) TestNewTaskSet(c *C) {
	ts0 := state.NewTaskSet()
	c.Check(ts0, HasLen, 0)

	st := state.New(nil)
	st.Lock()
	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	ts2 := state.NewTaskSet(t1, t2)
	st.Unlock()

	c.Check(ts2, HasLen, 2)
}

func (ts *taskSuite) TestTaskWaitAll(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	t3 := st.NewTask("setup", "3...")
	t3.WaitAll(state.NewTaskSet(t1, t2))

	c.Assert(t3.WaitTasks(), HasLen, 2)
	c.Assert(t3.Status(), Equals, state.WaitingStatus)

	c.Assert(t1.HaltTasks(), DeepEquals, []*state.Task{t3})
	c.Assert(t2.HaltTasks(), DeepEquals, []*state.Task{t3})
}

func (ts *taskSuite) TestTaskSetWaitFor(c *C) {
	st := state.New(nil)
	st.Lock()
	defer st.Unlock()

	t1 := st.NewTask("download", "1...")
	t2 := st.NewTask("install", "2...")
	t3 := st.NewTask("setup", "3...")
	ts23 := state.NewTaskSet(t2, t3)
	ts23.WaitFor(t1)

	c.Assert(t2.Status(), Equals, state.WaitingStatus)
	c.Assert(t2.WaitTasks(), DeepEquals, []*state.Task{t1})
	c.Assert(t3.Status(), Equals, state.WaitingStatus)
	c.Assert(t3.WaitTasks(), DeepEquals, []*state.Task{t1})

	c.Assert(t1.HaltTasks(), HasLen, 2)
}
