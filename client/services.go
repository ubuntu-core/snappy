// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2017 Canonical Ltd
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

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/systemd"
)

// ServicesOp encapsulate requests for performing an operation on a series of services
type ServiceOp struct {
	Services []string `json:"services,omitempty"`
	Action   string   `json:"action"`
}

func (op ServiceOp) Description() string {
	var verb string
	switch op.Action {
	case "enable-now":
		verb = i18n.G("Enable and start")
	case "disable-now":
		verb = i18n.G("Stop and disable")
	case "try-reload-or-restart":
		verb = i18n.G("Try to reload or restart")
	case "reload":
		// the following are spelled out so xgettext finds them
		verb = i18n.G("Reload")
	case "start":
		verb = i18n.G("Start")
	case "stop":
		verb = i18n.G("Stop")
	case "enable":
		verb = i18n.G("Enable")
	case "disable":
		verb = i18n.G("Disable")
	case "restart":
		verb = i18n.G("Restart")
	default:
		verb = strings.Title(op.Action)
	}

	if len(op.Services) == 0 {
		// not currently supported
		// TRANSLATORS: %s is the verb ("Stop", "Restart", etc)
		return fmt.Sprintf(i18n.G("%s all services."), verb)
	}

	// TRANSLATORS: first %s is the verb ("Stop", "Restart", etc),
	// second %s is list of service names ("a-snap.a-service, a-snap.b-service and %s")
	tpl := i18n.NG("%s service %s.", "%s services %s.", uint32(len(op.Services)%1000000))

	if len(op.Services) == 1 {
		return fmt.Sprintf(tpl, verb, op.Services[0])
	}

	return fmt.Sprintf(tpl, verb,
		// TRANSLATORS: first %s is a comma-separated list; second %s is the last element in the list
		fmt.Sprintf(i18n.G("%s and %s"), strings.Join(op.Services[:len(op.Services)-1], ", "), op.Services[len(op.Services)-1]))

}

// A Service is a description of a service's status in the system
type Service struct {
	Snap    string `json:"snap"`
	AppInfo        // note this is much less than snap.AppInfo, right now
	*systemd.ServiceStatus
	Logs []systemd.Log `json:"logs,omitempty"`
}

// helper for ServiceStatus and ServiceLogs
func (client *Client) serviceStatusAndLogs(serviceNames []string, logs bool) ([]Service, error) {
	query := url.Values{}
	query.Set("services", strings.Join(serviceNames, ","))
	if logs {
		query.Set("logs", "true")
	}
	var statuses []Service
	_, err := client.doSync("GET", "/v2/services", query, nil, nil, &statuses)
	if err != nil {
		return nil, err
	}
	return statuses, nil
}

// ServiceStatus asks for the status of a series of services, by name.
func (client *Client) ServiceStatus(serviceNames []string) ([]Service, error) {
	return client.serviceStatusAndLogs(serviceNames, false)
}

// ServiceLogs asks for the status and logs of a series of services, by name.
func (client *Client) ServiceLogs(serviceNames []string) ([]Service, error) {
	return client.serviceStatusAndLogs(serviceNames, true)
}

// ServiceOp asks to perform an operation on a series of services, by name.
func (client *Client) RunServiceOp(op *ServiceOp) (changeID string, err error) {
	buf, err := json.Marshal(op)
	if err != nil {
		return "", err
	}
	return client.doAsync("POST", "/v2/services", nil, nil, bytes.NewReader(buf))
}
