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

package hookstate

import (
	"regexp"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/snap"
)

type repositorySuite struct{}

var _ = Suite(&repositorySuite{})

func (s *repositorySuite) TestAddHandlerGenerator(c *C) {
	repository := newRepository()

	var calledContext *Context
	mockHandlerGenerator := func(context *Context) Handler {
		calledContext = context
		return newMockHandler()
	}

	// Verify that a handler generator can be added to the repository
	repository.addHandlerGenerator(regexp.MustCompile("test-hook"), mockHandlerGenerator)

	state := state.New(nil)
	state.Lock()
	task := state.NewTask("test-task", "my test task")
	state.Unlock()

	setup := hookSetup{Snap: "test-snap", Revision: snap.R(1), Hook: "test-hook"}
	context := newContext(task, setup)

	c.Assert(context, NotNil)

	// Verify that the handler can be generated
	handlers := repository.generateHandlers(context)
	c.Check(handlers, HasLen, 1)
	c.Check(calledContext, DeepEquals, context)

	// Add another handler
	repository.addHandlerGenerator(regexp.MustCompile(".*-hook"), mockHandlerGenerator)

	// Verify that two handlers are generated for the test-hook, now
	handlers = repository.generateHandlers(context)
	c.Check(handlers, HasLen, 2)
	c.Check(calledContext, DeepEquals, context)
}

type mockHandler struct {
	beforeCalled bool
	doneCalled   bool
	errorCalled  bool
	err          error
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		beforeCalled: false,
		doneCalled:   false,
		errorCalled:  false,
		err:          nil,
	}
}

func (h *mockHandler) Before() error {
	h.beforeCalled = true
	return nil
}

func (h *mockHandler) Done() error {
	h.doneCalled = true
	return nil
}

func (h *mockHandler) Error(err error) error {
	h.err = err
	h.errorCalled = true
	return nil
}
