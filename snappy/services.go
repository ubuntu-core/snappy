// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
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

package snappy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/ubuntu-core/snappy/arch"
	"github.com/ubuntu-core/snappy/dirs"
	"github.com/ubuntu-core/snappy/logger"
	"github.com/ubuntu-core/snappy/osutil"
	"github.com/ubuntu-core/snappy/snap"
	"github.com/ubuntu-core/snappy/snap/snapenv"
	"github.com/ubuntu-core/snappy/systemd"
	"github.com/ubuntu-core/snappy/timeout"
)

type agreer interface {
	Agreed(intro, license string) bool
}

type interacter interface {
	agreer
	Notify(status string)
}

// wait this time between TERM and KILL
var killWait = 5 * time.Second

const (
	// the default target for systemd units that we generate
	servicesSystemdTarget = "multi-user.target"

	// the default target for systemd units that we generate
	socketsSystemdTarget = "sockets.target"
)

// servicesBinariesStringsWhitelist is the whitelist of legal chars
// in the "binaries" and "services" section of the snap.yaml
var servicesBinariesStringsWhitelist = regexp.MustCompile(`^[A-Za-z0-9/. _#:-]*$`)

func serviceStopTimeout(app *snap.AppInfo) time.Duration {
	tout := app.StopTimeout
	if tout == 0 {
		tout = timeout.DefaultTimeout
	}
	return time.Duration(tout)
}

func generateSnapServicesFile(app *snap.AppInfo, baseDir string) (string, error) {
	if err := snap.ValidateApp(app); err != nil {
		return "", err
	}

	desc := fmt.Sprintf("service %s for snap %s - autogenerated DO NO EDIT", app.Name, app.Snap.Name())

	socketFileName := ""
	if app.Socket {
		socketFileName = filepath.Base(app.ServiceSocketFile())
	}

	return GenServiceFile(&ServiceDescription{
		SnapName:       app.Snap.Name(),
		AppName:        app.Name,
		Version:        app.Snap.Version,
		Revision:       app.Snap.Revision,
		Description:    desc,
		SnapPath:       baseDir,
		Start:          app.Command,
		Stop:           app.Stop,
		PostStop:       app.PostStop,
		StopTimeout:    serviceStopTimeout(app),
		AaProfile:      app.SecurityTag(),
		BusName:        app.BusName,
		Type:           app.Daemon,
		UdevAppName:    app.SecurityTag(),
		Socket:         app.Socket,
		SocketFileName: socketFileName,
		Restart:        app.RestartCond,
	}), nil
}

func generateSnapSocketFile(app *snap.AppInfo, baseDir string) (string, error) {
	if err := snap.ValidateApp(app); err != nil {
		return "", err
	}

	// lp: #1515709, systemd will default to 0666 if no socket mode
	// is specified
	if app.SocketMode == "" {
		app.SocketMode = "0660"
	}

	serviceFileName := filepath.Base(app.ServiceFile())

	return GenSocketFile(&ServiceDescription{
		ServiceFileName: serviceFileName,
		ListenStream:    app.ListenStream,
		SocketMode:      app.SocketMode,
	}), nil
}

func addPackageServices(s *snap.Info, inter interacter) error {
	baseDir := s.MountDir()

	for _, app := range s.Apps {
		if app.Daemon == "" {
			continue
		}

		// this will remove the global base dir when generating the
		// service file, this ensures that /snap/foo/1.0/bin/start
		// is in the service file when the SetRoot() option
		// is used
		realBaseDir := stripGlobalRootDir(baseDir)
		// Generate service file
		content, err := generateSnapServicesFile(app, realBaseDir)
		if err != nil {
			return err
		}
		svcFilePath := app.ServiceFile()
		os.MkdirAll(filepath.Dir(svcFilePath), 0755)
		if err := osutil.AtomicWriteFile(svcFilePath, []byte(content), 0644, 0); err != nil {
			return err
		}
		// Generate systemd socket file if needed
		if app.Socket {
			content, err := generateSnapSocketFile(app, realBaseDir)
			if err != nil {
				return err
			}
			svcSocketFilePath := app.ServiceSocketFile()
			os.MkdirAll(filepath.Dir(svcSocketFilePath), 0755)
			if err := osutil.AtomicWriteFile(svcSocketFilePath, []byte(content), 0644, 0); err != nil {
				return err
			}
		}
		// daemon-reload and enable plus start
		serviceName := filepath.Base(app.ServiceFile())
		sysd := systemd.New(dirs.GlobalRootDir, inter)

		if err := sysd.DaemonReload(); err != nil {
			return err
		}

		// enable the service
		if err := sysd.Enable(serviceName); err != nil {
			return err
		}

		if err := sysd.Start(serviceName); err != nil {
			return err
		}

		if app.Socket {
			socketName := filepath.Base(app.ServiceSocketFile())
			// enable the socket
			if err := sysd.Enable(socketName); err != nil {
				return err
			}

			if err := sysd.Start(socketName); err != nil {
				return err
			}
		}
	}

	return nil
}

func removePackageServices(s *snap.Info, inter interacter) error {
	sysd := systemd.New(dirs.GlobalRootDir, inter)

	nservices := 0

	for _, app := range s.Apps {
		if app.Daemon == "" {
			continue
		}
		nservices++

		serviceName := filepath.Base(app.ServiceFile())
		if err := sysd.Disable(serviceName); err != nil {
			return err
		}
		if err := sysd.Stop(serviceName, serviceStopTimeout(app)); err != nil {
			if !systemd.IsTimeout(err) {
				return err
			}
			inter.Notify(fmt.Sprintf("%s refused to stop, killing.", serviceName))
			// ignore errors for kill; nothing we'd do differently at this point
			sysd.Kill(serviceName, "TERM")
			time.Sleep(killWait)
			sysd.Kill(serviceName, "KILL")
		}

		if err := os.Remove(app.ServiceFile()); err != nil && !os.IsNotExist(err) {
			logger.Noticef("Failed to remove service file for %q: %v", serviceName, err)
		}

		if err := os.Remove(app.ServiceSocketFile()); err != nil && !os.IsNotExist(err) {
			logger.Noticef("Failed to remove socket file for %q: %v", serviceName, err)
		}
	}

	// only reload if we actually had services
	if nservices > 0 {
		if err := sysd.DaemonReload(); err != nil {
			return err
		}
	}

	return nil
}

// ServiceDescription describes a snappy systemd service
type ServiceDescription struct {
	SnapName        string
	AppName         string
	Version         string
	Revision        int
	Description     string
	SnapPath        string
	Start           string
	Stop            string
	PostStop        string
	StopTimeout     time.Duration
	Restart         systemd.RestartCondition
	Type            string
	AaProfile       string
	BusName         string
	UdevAppName     string
	Socket          bool
	SocketFileName  string
	ListenStream    string
	SocketMode      string
	ServiceFileName string
}

func GenServiceFile(desc *ServiceDescription) string {
	serviceTemplate := `[Unit]
Description={{.Description}}
After=snapd.frameworks.target{{ if .Socket }} {{.SocketFileName}}{{end}}
Requires=snapd.frameworks.target{{ if .Socket }} {{.SocketFileName}}{{end}}
X-Snappy=yes

[Service]
ExecStart=/usr/bin/ubuntu-core-launcher {{.UdevAppName}} {{.AaProfile}} {{.FullPathStart}}
Restart={{.Restart}}
WorkingDirectory=/var{{.SnapPath}}
Environment={{.EnvVars}}
{{if .Stop}}ExecStop=/usr/bin/ubuntu-core-launcher {{.UdevAppName}} {{.AaProfile}} {{.FullPathStop}}{{end}}
{{if .PostStop}}ExecStopPost=/usr/bin/ubuntu-core-launcher {{.UdevAppName}} {{.AaProfile}} {{.FullPathPostStop}}{{end}}
{{if .StopTimeout}}TimeoutStopSec={{.StopTimeout.Seconds}}{{end}}
Type={{.Type}}
{{if .BusName}}BusName={{.BusName}}{{end}}

[Install]
WantedBy={{.ServiceSystemdTarget}}
`
	var templateOut bytes.Buffer
	t := template.Must(template.New("wrapper").Parse(serviceTemplate))

	restartCond := desc.Restart.String()
	if restartCond == "" {
		restartCond = systemd.RestartOnFailure.String()
	}

	wrapperData := struct {
		// the service description
		ServiceDescription
		// and some composed values
		FullPathStart        string
		FullPathStop         string
		FullPathPostStop     string
		ServiceSystemdTarget string
		SnapArch             string
		Home                 string
		EnvVars              string
		SocketFileName       string
		Restart              string
		Type                 string
	}{
		*desc,
		filepath.Join(desc.SnapPath, desc.Start),
		filepath.Join(desc.SnapPath, desc.Stop),
		filepath.Join(desc.SnapPath, desc.PostStop),
		servicesSystemdTarget,
		arch.UbuntuArchitecture(),
		// systemd runs as PID 1 so %h will not work.
		"/root",
		"",
		desc.SocketFileName,
		restartCond,
		desc.Type,
	}
	allVars := snapenv.GetBasicSnapEnvVars(wrapperData)
	allVars = append(allVars, snapenv.GetUserSnapEnvVars(wrapperData)...)
	wrapperData.EnvVars = "\"" + strings.Join(allVars, "\" \"") + "\"" // allVars won't be empty

	if err := t.Execute(&templateOut, wrapperData); err != nil {
		// this can never happen, except we forget a variable
		logger.Panicf("Unable to execute template: %v", err)
	}

	return templateOut.String()
}

func GenSocketFile(desc *ServiceDescription) string {
	serviceTemplate := `[Unit]
Description={{.Description}} Socket Unit File
PartOf={{.ServiceFileName}}
X-Snappy=yes

[Socket]
ListenStream={{.ListenStream}}
{{if .SocketMode}}SocketMode={{.SocketMode}}{{end}}

[Install]
WantedBy={{.SocketSystemdTarget}}
`
	var templateOut bytes.Buffer
	t := template.Must(template.New("wrapper").Parse(serviceTemplate))

	wrapperData := struct {
		// the service description
		ServiceDescription
		// and some composed values
		ServiceFileName,
		ListenStream string
		SocketMode          string
		SocketSystemdTarget string
	}{
		*desc,
		desc.ServiceFileName,
		desc.ListenStream,
		desc.SocketMode,
		socketsSystemdTarget,
	}

	if err := t.Execute(&templateOut, wrapperData); err != nil {
		// this can never happen, except we forget a variable
		logger.Panicf("Unable to execute template: %v", err)
	}

	return templateOut.String()
}
