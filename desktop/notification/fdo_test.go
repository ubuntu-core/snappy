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

package notification_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus"
	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/desktop/notification"
	"github.com/snapcore/snapd/desktop/notification/notificationtest"
	"github.com/snapcore/snapd/testutil"
)

type fdoSuite struct {
	testutil.BaseTest
	testutil.DBusTest

	backend *notificationtest.FdoServer
}

var _ = Suite(&fdoSuite{})

func (s *fdoSuite) SetUpTest(c *C) {
	s.BaseTest.SetUpTest(c)
	s.DBusTest.SetUpTest(c)

	backend, err := notificationtest.NewFdoServer()
	c.Assert(err, IsNil)
	s.AddCleanup(func() { c.Check(backend.Stop(), IsNil) })
	s.backend = backend
}

func (s *fdoSuite) TearDownTest(c *C) {
	s.DBusTest.TearDownTest(c)
	s.BaseTest.TearDownTest(c)
}

func (s *fdoSuite) TestServerInformationSuccess(c *C) {
	srv := notification.New(s.SessionBus)
	name, vendor, version, specVersion, err := srv.ServerInformation()
	c.Assert(err, IsNil)
	c.Check(name, Equals, "name")
	c.Check(vendor, Equals, "vendor")
	c.Check(version, Equals, "version")
	c.Check(specVersion, Equals, "specVersion")
}

func (s *fdoSuite) TestServerInformationError(c *C) {
	s.backend.SetError(&dbus.Error{Name: "org.freedesktop.DBus.Error.Failed"})
	srv := notification.New(s.SessionBus)
	_, _, _, _, err := srv.ServerInformation()
	c.Assert(err, ErrorMatches, "org.freedesktop.DBus.Error.Failed")
}

func (s *fdoSuite) TestServerCapabilitiesSuccess(c *C) {
	srv := notification.New(s.SessionBus)
	caps, err := srv.ServerCapabilities()
	c.Assert(err, IsNil)
	c.Check(caps, DeepEquals, []notification.ServerCapability{"cap-foo", "cap-bar"})
}

func (s *fdoSuite) TestServerCapabilitiesError(c *C) {
	s.backend.SetError(&dbus.Error{Name: "org.freedesktop.DBus.Error.Failed"})
	srv := notification.New(s.SessionBus)
	_, err := srv.ServerCapabilities()
	c.Assert(err, ErrorMatches, "org.freedesktop.DBus.Error.Failed")
}

func (s *fdoSuite) TestSendNotificationSuccess(c *C) {
	srv := notification.New(s.SessionBus)
	id, err := srv.SendNotification(&notification.Message{
		AppName:       "app-name",
		Icon:          "icon",
		Summary:       "summary",
		Body:          "body",
		ExpireTimeout: time.Second * 1,
		ReplacesID:    notification.ID(42),
		Actions: []notification.Action{
			{ActionKey: "key-1", LocalizedText: "text-1"},
			{ActionKey: "key-2", LocalizedText: "text-2"},
		},
		Hints: []notification.Hint{
			{Name: "hint-str", Value: "str"},
			{Name: "hint-bool", Value: true},
		},
	})
	c.Assert(err, IsNil)

	c.Check(s.backend.Get(uint32(id)), DeepEquals, &notificationtest.FdoNotification{
		ID:      uint32(id),
		AppName: "app-name",
		Icon:    "icon",
		Summary: "summary",
		Body:    "body",
		Actions: []string{"key-1", "text-1", "key-2", "text-2"},
		Hints: map[string]dbus.Variant{
			"hint-str":  dbus.MakeVariant("str"),
			"hint-bool": dbus.MakeVariant(true),
		},
		Expires: 1000,
	})
}

func (s *fdoSuite) TestSendNotificationWithServerDecidedExpireTimeout(c *C) {
	srv := notification.New(s.SessionBus)
	id, err := srv.SendNotification(&notification.Message{
		ExpireTimeout: notification.ServerSelectedExpireTimeout,
	})
	c.Assert(err, IsNil)

	c.Check(s.backend.Get(uint32(id)), DeepEquals, &notificationtest.FdoNotification{
		ID:      uint32(id),
		Actions: []string{},
		Hints:   map[string]dbus.Variant{},
		Expires: -1,
	})
}

func (s *fdoSuite) TestSendNotificationError(c *C) {
	s.backend.SetError(&dbus.Error{Name: "org.freedesktop.DBus.Error.Failed"})
	srv := notification.New(s.SessionBus)
	_, err := srv.SendNotification(&notification.Message{})
	c.Assert(err, ErrorMatches, "org.freedesktop.DBus.Error.Failed")
}

func (s *fdoSuite) TestCloseNotificationSuccess(c *C) {
	srv := notification.New(s.SessionBus)
	id, err := srv.SendNotification(&notification.Message{})
	c.Assert(err, IsNil)

	err = srv.CloseNotification(id)
	c.Assert(err, IsNil)
	c.Check(s.backend.Get(uint32(id)), IsNil)
}

func (s *fdoSuite) TestCloseNotificationError(c *C) {
	s.backend.SetError(&dbus.Error{Name: "org.freedesktop.DBus.Error.Failed"})
	srv := notification.New(s.SessionBus)
	err := srv.CloseNotification(notification.ID(42))
	c.Assert(err, ErrorMatches, "org.freedesktop.DBus.Error.Failed")
}

type testObserver struct {
	notificationClosed func(notification.ID, notification.CloseReason) error
	actionInvoked      func(notification.ID, string) error
}

func (o *testObserver) NotificationClosed(id notification.ID, reason notification.CloseReason) error {
	if o.notificationClosed != nil {
		return o.notificationClosed(id, reason)
	}
	return nil
}

func (o *testObserver) ActionInvoked(id notification.ID, actionKey string) error {
	if o.actionInvoked != nil {
		return o.actionInvoked(id, actionKey)
	}
	return nil
}

func (s *fdoSuite) TestObserveNotificationsContextAndSignalWatch(c *C) {
	srv := notification.New(s.SessionBus)

	ctx, cancel := context.WithCancel(context.TODO())
	signalDelivered := make(chan struct{}, 1)
	defer close(signalDelivered)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := srv.ObserveNotifications(ctx, &testObserver{
			actionInvoked: func(id notification.ID, actionKey string) error {
				select {
				case signalDelivered <- struct{}{}:
				default:
				}
				return nil
			},
		})
		c.Assert(err, ErrorMatches, "context canceled")
		wg.Done()
	}()
	// Send signals until we've got confirmation that the observer
	// is firing
	for sendSignal := true; sendSignal; {
		c.Check(s.backend.InvokeAction(42, "action-key"), IsNil)
		select {
		case <-signalDelivered:
			sendSignal = false
		default:
		}
	}
	cancel()
	// Wait for ObserveNotifications to return
	wg.Wait()
}

func (s *fdoSuite) TestObserveNotificationsProcessingError(c *C) {
	srv := notification.New(s.SessionBus)

	signalDelivered := make(chan struct{}, 1)
	defer close(signalDelivered)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := srv.ObserveNotifications(context.TODO(), &testObserver{
			actionInvoked: func(id notification.ID, actionKey string) error {
				signalDelivered <- struct{}{}
				c.Check(id, Equals, notification.ID(42))
				c.Check(actionKey, Equals, "action-key")
				return fmt.Errorf("boom")
			},
		})
		c.Log("End of goroutine")
		c.Check(err, ErrorMatches, "cannot process ActionInvoked signal: boom")
		wg.Done()
	}()
	// We don't know if the other goroutine has set up the signal
	// match yet, so send signals until we get confirmation.
	for sendSignal := true; sendSignal; {
		c.Check(s.backend.InvokeAction(42, "action-key"), IsNil)
		select {
		case <-signalDelivered:
			sendSignal = false
		default:
		}
	}
	// Wait for ObserveNotifications to return
	wg.Wait()
}

func (s *fdoSuite) TestProcessActionInvokedSignalSuccess(c *C) {
	called := false
	err := notification.ProcessSignal(&dbus.Signal{
		// Sender and Path are not used
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{uint32(42), "action-key"},
	}, &testObserver{
		actionInvoked: func(id notification.ID, actionKey string) error {
			called = true
			c.Check(id, Equals, notification.ID(42))
			c.Check(actionKey, Equals, "action-key")
			return nil
		},
	})
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
}

func (s *fdoSuite) TestProcessActionInvokedSignalError(c *C) {
	err := notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{uint32(42), "action-key"},
	}, &testObserver{
		actionInvoked: func(id notification.ID, actionKey string) error {
			return fmt.Errorf("boom")
		},
	})
	c.Assert(err, ErrorMatches, "cannot process ActionInvoked signal: boom")
}

func (s *fdoSuite) TestProcessActionInvokedSignalBodyParseErrors(c *C) {
	err := notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{uint32(42), "action-key", "unexpected"},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process ActionInvoked signal: unexpected number of body elements: 3")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{uint32(42)},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process ActionInvoked signal: unexpected number of body elements: 1")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{uint32(42), true},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process ActionInvoked signal: expected second body element to be string, got bool")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.ActionInvoked",
		Body: []interface{}{true, "action-key"},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process ActionInvoked signal: expected first body element to be uint32, got bool")
}

func (s *fdoSuite) TestProcessNotificationClosedSignalSuccess(c *C) {
	called := false
	err := notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{uint32(42), uint32(2)},
	}, &testObserver{
		notificationClosed: func(id notification.ID, reason notification.CloseReason) error {
			called = true
			c.Check(id, Equals, notification.ID(42))
			c.Check(reason, Equals, notification.CloseReason(2))
			return nil
		},
	})
	c.Assert(err, IsNil)
	c.Assert(called, Equals, true)
}

func (s *fdoSuite) TestProcessNotificationClosedSignalError(c *C) {
	err := notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{uint32(42), uint32(2)},
	}, &testObserver{
		notificationClosed: func(id notification.ID, reason notification.CloseReason) error {
			return fmt.Errorf("boom")
		},
	})
	c.Assert(err, ErrorMatches, "cannot process NotificationClosed signal: boom")
}

func (s *fdoSuite) TestProcessNotificationClosedSignalBodyParseErrors(c *C) {
	err := notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{uint32(42), uint32(2), "unexpected"},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process NotificationClosed signal: unexpected number of body elements: 3")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{uint32(42)},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process NotificationClosed signal: unexpected number of body elements: 1")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{uint32(42), true},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process NotificationClosed signal: expected second body element to be uint32, got bool")

	err = notification.ProcessSignal(&dbus.Signal{
		Name: "org.freedesktop.Notifications.NotificationClosed",
		Body: []interface{}{true, uint32(2)},
	}, &testObserver{})
	c.Assert(err, ErrorMatches, "cannot process NotificationClosed signal: expected first body element to be uint32, got bool")
}
