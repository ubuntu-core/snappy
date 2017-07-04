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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"gopkg.in/retry.v1"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/httputil"
	"github.com/snapcore/snapd/osutil"
)

// Runner implements fetching, tracking and running repairs.
type Runner struct {
	BaseURL *url.URL
	cli     *http.Client

	state state
}

// NewRunner returns a Runner.
func NewRunner() *Runner {
	// TODO: pass TLSConfig with lower-bounded time
	opts := httputil.ClientOpts{
		Timeout:    15 * time.Second,
		MayLogBody: false,
	}
	cli := httputil.NewHTTPClient(&opts)
	return &Runner{
		cli: cli,
	}
	// TODO: call LoadState implicitly?
}

var (
	fetchRetryStrategy = retry.LimitCount(10, retry.LimitTime(1*time.Minute,
		retry.Exponential{
			Initial: 100 * time.Millisecond,
			Factor:  2.5,
		},
	))

	peekRetryStrategy = retry.LimitCount(7, retry.LimitTime(30*time.Second,
		retry.Exponential{
			Initial: 100 * time.Millisecond,
			Factor:  2.5,
		},
	))
)

var ErrRepairNotFound = errors.New("repair not found")

var (
	maxRepairScriptSize = 24 * 1024 * 1024
)

// Fetch retrieves a stream with the repair with the given ids and any auxiliary assertions.
func (run *Runner) Fetch(brandID, repairID string) (r []asserts.Assertion, err error) {
	u, err := run.BaseURL.Parse(fmt.Sprintf("repairs/%s/%s", brandID, repairID))
	if err != nil {
		return nil, err
	}

	resp, err := httputil.RetryRequest(u.String(), func() (*http.Response, error) {
		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/x.ubuntu.assertion")
		return run.cli.Do(req)
	}, func(resp *http.Response) error {
		if resp.StatusCode == 200 {
			// decode assertions
			dec := asserts.NewDecoderWithTypeMaxBodySize(resp.Body, map[*asserts.AssertionType]int{
				asserts.RepairType: maxRepairScriptSize,
			})
			for {
				a, err := dec.Decode()
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				r = append(r, a)
			}
			if len(r) == 0 {
				return io.ErrUnexpectedEOF
			}
		}
		return nil
	}, fetchRetryStrategy)

	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case 200:
		// ok
	case 404:
		return nil, ErrRepairNotFound
	default:
		return nil, fmt.Errorf("cannot fetch repair, unexpected status %d", resp.StatusCode)
	}

	repair, ok := r[0].(*asserts.Repair)
	if !ok {
		return nil, fmt.Errorf("cannot fetch repair, unexpected first assertion %q", r[0].Type().Name)
	}

	if repair.BrandID() != brandID || repair.RepairID() != repairID {
		return nil, fmt.Errorf("cannot fetch repair, id mismatch %s/%s != %s/%s", repair.BrandID(), repair.RepairID(), brandID, repairID)
	}

	return r, nil
}

type peekResp struct {
	Headers map[string]interface{} `json:"headers"`
}

// Peek retrieves the headers for the repair with the given ids.
func (run *Runner) Peek(brandID, repairID string) (headers map[string]interface{}, err error) {
	u, err := run.BaseURL.Parse(fmt.Sprintf("repairs/%s/%s", brandID, repairID))
	if err != nil {
		return nil, err
	}

	var rsp peekResp

	resp, err := httputil.RetryRequest(u.String(), func() (*http.Response, error) {
		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		return run.cli.Do(req)
	}, func(resp *http.Response) error {
		rsp.Headers = nil
		if resp.StatusCode == 200 {
			dec := json.NewDecoder(resp.Body)
			return dec.Decode(&rsp)
		}
		return nil
	}, peekRetryStrategy)

	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case 200:
		// ok
	case 404:
		return nil, ErrRepairNotFound
	default:
		return nil, fmt.Errorf("cannot peek repair headers, unexpected status %d", resp.StatusCode)
	}

	headers = rsp.Headers
	if headers["brand-id"] != brandID || headers["repair-id"] != repairID {
		return nil, fmt.Errorf("cannot peek repair headers, id mismatch %s/%s != %s/%s", headers["brand-id"], headers["repair-id"], brandID, repairID)
	}

	return headers, nil
}

// deviceInfo captures information about the device.
type deviceInfo struct {
	Brand string `json:"brand"`
	Model string `json:"model"`
}

// repairStatus represents the possible statuses of a repair.
type repairStatus int

const (
	RetryStatus repairStatus = iota
	NotApplicableStatus
	DoneStatus
)

// repairState holds the current revision and status of a repair in a sequence of repairs.
type repairState struct {
	Seq      int          `json:"seq"`
	Revision int          `json:"revision"`
	Status   repairStatus `json:"status"`
}

// state holds the atomically updated control state of the runner with sequences of repairs and their states.
type state struct {
	Device    deviceInfo                `json:"device"`
	Sequences map[string][]*repairState `json:"sequences,omitempty"`
}

func (run *Runner) readState() error {
	r, err := os.Open(dirs.SnapRepairStateFile)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(r)
	return dec.Decode(&run.state)
}

func (run *Runner) initState() error {
	if err := os.MkdirAll(dirs.SnapRepairDir, 0775); err != nil {
		panic(fmt.Sprintf("cannot create repair state directory: %v", err))
	}
	// best-effort remove old
	os.Remove(dirs.SnapRepairStateFile)
	// TODO: init Device
	run.SaveState()
	return nil
}

// LoadState loads the repairs' state from disk, and (re)initializes it if it's missing or corrupted.
func (run *Runner) LoadState() error {
	err := run.readState()
	if err == nil {
		return nil
	}
	// error => initialize from scratch
	// TODO: log error?
	return run.initState()
}

// SaveState saves the repairs' state to disk.
func (run *Runner) SaveState() {
	m, err := json.Marshal(&run.state)
	if err != nil {
		panic(fmt.Sprintf("cannot marshal repair state: %v", err))
	}
	err = osutil.AtomicWriteFile(dirs.SnapRepairStateFile, m, 0600, 0)
	if err != nil {
		panic(fmt.Sprintf("cannot save repair state: %v", err))
	}
}
