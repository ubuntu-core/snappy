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

package main_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	. "gopkg.in/check.v1"
	"gopkg.in/retry.v1"

	"github.com/snapcore/snapd/asserts"
	repair "github.com/snapcore/snapd/cmd/snap-repair"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/osutil"
)

type runnerSuite struct {
	tmpdir string
}

var _ = Suite(&runnerSuite{})

func (s *runnerSuite) SetUpTest(c *C) {
	s.tmpdir = c.MkDir()
	dirs.SetRootDir(s.tmpdir)
}

var (
	testKey = `type: account-key
authority-id: canonical
account-id: canonical
name: repair
public-key-sha3-384: KPIl7M4vQ9d4AUjkoU41TGAwtOMLc_bWUCeW8AvdRWD4_xcP60Oo4ABsFNo6BtXj
since: 2015-11-16T15:04:00Z
body-length: 149
sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij

AcZrBFaFwYABAvCX5A8dTcdLdhdiuy2YRHO5CAfM5InQefkKOhNMUq2yfi3Sk6trUHxskhZkPnm4
NKx2yRr332q7AJXQHLX+DrZ29ycyoQ2NQGO3eAfQ0hjAAQFYBF8SSh5SutPu5XCVABEBAAE=

AXNpZw==
`

	testRepair = `type: repair
authority-id: canonical
brand-id: canonical
repair-id: 2
architectures:
  - amd64
  - arm64
series:
  - 16
models:
  - xyz/frobinator
timestamp: 2017-03-30T12:22:16Z
body-length: 7
sign-key-sha3-384: KPIl7M4vQ9d4AUjkoU41TGAwtOMLc_bWUCeW8AvdRWD4_xcP60Oo4ABsFNo6BtXj

script


AXNpZw==
`
	testHeadersResp = `{"headers":
{"architectures":["amd64","arm64"],"authority-id":"canonical","body-length":"7","brand-id":"canonical","models":["xyz/frobinator"],"repair-id":"2","series":["16"],"sign-key-sha3-384":"KPIl7M4vQ9d4AUjkoU41TGAwtOMLc_bWUCeW8AvdRWD4_xcP60Oo4ABsFNo6BtXj","timestamp":"2017-03-30T12:22:16Z","type":"repair"}}`
)

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func (s *runnerSuite) TestFetchJustRepair(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/x.ubuntu.assertion")
		c.Check(r.URL.Path, Equals, "/repairs/canonical/2")
		io.WriteString(w, testRepair)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	a, err := runner.Fetch("canonical", "2")
	c.Assert(err, IsNil)
	c.Check(a, HasLen, 1)
	_, ok := a[0].(*asserts.Repair)
	c.Check(ok, Equals, true)
}

func (s *runnerSuite) TestFetchScriptTooBig(c *C) {
	restore := repair.MockMaxRepairScriptSize(4)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		c.Check(r.Header.Get("Accept"), Equals, "application/x.ubuntu.assertion")
		c.Check(r.URL.Path, Equals, "/repairs/canonical/2")
		io.WriteString(w, testRepair)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, ErrorMatches, `assertion body length 7 exceeds maximum body size 4 for "repair".*`)
	c.Assert(n, Equals, 1)
}

var (
	testRetryStrategy = retry.LimitCount(5, retry.LimitTime(1*time.Second,
		retry.Exponential{
			Initial: 1 * time.Millisecond,
			Factor:  1,
		},
	))
)

func (s *runnerSuite) TestFetch500(c *C) {
	restore := repair.MockFetchRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(500)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, ErrorMatches, "cannot fetch repair, unexpected status 500")
	c.Assert(n, Equals, 5)
}

func (s *runnerSuite) TestFetchEmpty(c *C) {
	restore := repair.MockFetchRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(200)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, Equals, io.ErrUnexpectedEOF)
	c.Assert(n, Equals, 5)
}

func (s *runnerSuite) TestFetchBroken(c *C) {
	restore := repair.MockFetchRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(200)
		io.WriteString(w, "xyz:")
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, Equals, io.ErrUnexpectedEOF)
	c.Assert(n, Equals, 5)
}

func (s *runnerSuite) TestFetchNotFound(c *C) {
	restore := repair.MockFetchRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(404)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, Equals, repair.ErrRepairNotFound)
	c.Assert(n, Equals, 1)
}

func (s *runnerSuite) TestFetchIdMismatch(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/x.ubuntu.assertion")
		io.WriteString(w, testRepair)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "4")
	c.Assert(err, ErrorMatches, `cannot fetch repair, id mismatch canonical/2 != canonical/4`)
}

func (s *runnerSuite) TestFetchWrongFirstType(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/x.ubuntu.assertion")
		c.Check(r.URL.Path, Equals, "/repairs/canonical/2")
		io.WriteString(w, testKey)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Fetch("canonical", "2")
	c.Assert(err, ErrorMatches, `cannot fetch repair, unexpected first assertion "account-key"`)
}

func (s *runnerSuite) TestFetchRepairPlusKey(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/x.ubuntu.assertion")
		c.Check(r.URL.Path, Equals, "/repairs/canonical/2")
		io.WriteString(w, testRepair)
		io.WriteString(w, "\n")
		io.WriteString(w, testKey)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	a, err := runner.Fetch("canonical", "2")
	c.Assert(err, IsNil)
	c.Check(a, HasLen, 2)
	_, ok := a[0].(*asserts.Repair)
	c.Check(ok, Equals, true)
	_, ok = a[1].(*asserts.AccountKey)
	c.Check(ok, Equals, true)
}

func (s *runnerSuite) TestPeek(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/json")
		c.Check(r.URL.Path, Equals, "/repairs/canonical/2")
		io.WriteString(w, testHeadersResp)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	h, err := runner.Peek("canonical", "2")
	c.Assert(err, IsNil)
	c.Check(h["series"], DeepEquals, []interface{}{"16"})
	c.Check(h["architectures"], DeepEquals, []interface{}{"amd64", "arm64"})
	c.Check(h["models"], DeepEquals, []interface{}{"xyz/frobinator"})
}

func (s *runnerSuite) TestPeek500(c *C) {
	restore := repair.MockPeekRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(500)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Peek("canonical", "2")
	c.Assert(err, ErrorMatches, "cannot peek repair headers, unexpected status 500")
	c.Assert(n, Equals, 5)
}

func (s *runnerSuite) TestPeekInvalid(c *C) {
	restore := repair.MockPeekRetryStrategy(testRetryStrategy)
	defer restore()

	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(200)
		io.WriteString(w, "{")
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Peek("canonical", "2")
	c.Assert(err, Equals, io.ErrUnexpectedEOF)
	c.Assert(n, Equals, 5)
}

func (s *runnerSuite) TestPeekNotFound(c *C) {
	n := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(404)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Peek("canonical", "2")
	c.Assert(err, Equals, repair.ErrRepairNotFound)
	c.Assert(n, Equals, 1)
}

func (s *runnerSuite) TestPeekIdMismatch(c *C) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Header.Get("Accept"), Equals, "application/json")
		io.WriteString(w, testHeadersResp)
	}))

	c.Assert(mockServer, NotNil)
	defer mockServer.Close()

	runner := repair.NewRunner()
	runner.BaseURL = mustParseURL(mockServer.URL)

	_, err := runner.Peek("canonical", "4")
	c.Assert(err, ErrorMatches, `cannot peek repair headers, id mismatch canonical/2 != canonical/4`)
}

func (s *runnerSuite) TestLoadState(c *C) {
	err := os.MkdirAll(dirs.SnapRepairDir, 0775)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(dirs.SnapRepairStateFile, []byte(`{"device": {"brand":"my-brand","model":"my-model"}}`), 0600)
	c.Assert(err, IsNil)
	runner := repair.NewRunner()
	err = runner.LoadState()
	c.Assert(err, IsNil)
	c.Check(runner.Brand(), Equals, "my-brand")
	c.Check(runner.Model(), Equals, "my-model")
}

func (s *runnerSuite) TestLoadStateInitState(c *C) {
	// sanity
	c.Check(osutil.IsDirectory(dirs.SnapRepairDir), Equals, false)
	c.Check(osutil.FileExists(dirs.SnapRepairStateFile), Equals, false)
	runner := repair.NewRunner()
	err := runner.LoadState()
	c.Assert(err, IsNil)
	c.Check(osutil.FileExists(dirs.SnapRepairStateFile), Equals, true)
	// TODO: init state will do more later
	c.Check(runner.Brand(), Equals, "")
	c.Check(runner.Model(), Equals, "")
}

func (s *runnerSuite) TestLoadStateInitStateFail(c *C) {
	err := os.Chmod(s.tmpdir, 0555)
	c.Assert(err, IsNil)

	runner := repair.NewRunner()
	c.Check(runner.LoadState, PanicMatches, `cannot create repair state directory:.*`)
}

func (s *runnerSuite) TestSaveStateFail(c *C) {
	runner := repair.NewRunner()
	err := runner.LoadState()
	c.Assert(err, IsNil)

	err = os.Chmod(dirs.SnapRepairDir, 0555)
	c.Assert(err, IsNil)
	defer os.Chmod(dirs.SnapRepairDir, 0775)

	c.Check(runner.SaveState, PanicMatches, `cannot save repair state:.*`)
}
