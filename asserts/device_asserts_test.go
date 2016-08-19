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

package asserts_test

import (
	"strings"
	"time"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/asserts"
)

type modelSuite struct {
	ts     time.Time
	tsLine string
}

var (
	_ = Suite(&modelSuite{})
	_ = Suite(&serialSuite{})
)

func (mods *modelSuite) SetUpSuite(c *C) {
	mods.ts = time.Now().Truncate(time.Second).UTC()
	mods.tsLine = "timestamp: " + mods.ts.Format(time.RFC3339) + "\n"
}

const modelExample = "type: model\n" +
	"authority-id: brand-id1\n" +
	"series: 16\n" +
	"brand-id: brand-id1\n" +
	"model: baz-3000\n" +
	"core: core\n" +
	"architecture: amd64\n" +
	"gadget: brand-gadget\n" +
	"kernel: baz-linux\n" +
	"store: brand-store\n" +
	"allowed-modes: \n" +
	"required-snaps: foo, bar\n" +
	"class: fixed\n" +
	"TSLINE" +
	"body-length: 0\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij" +
	"\n\n" +
	"AXNpZw=="

func (mods *modelSuite) TestDecodeOK(c *C) {
	encoded := strings.Replace(modelExample, "TSLINE", mods.tsLine, 1)
	a, err := asserts.Decode([]byte(encoded))
	c.Assert(err, IsNil)
	c.Check(a.Type(), Equals, asserts.ModelType)
	model := a.(*asserts.Model)
	c.Check(model.AuthorityID(), Equals, "brand-id1")
	c.Check(model.Timestamp(), Equals, mods.ts)
	c.Check(model.Series(), Equals, "16")
	c.Check(model.BrandID(), Equals, "brand-id1")
	c.Check(model.Model(), Equals, "baz-3000")
	c.Check(model.Class(), Equals, "fixed")
	c.Check(model.Core(), Equals, "core")
	c.Check(model.Architecture(), Equals, "amd64")
	c.Check(model.Gadget(), Equals, "brand-gadget")
	c.Check(model.Kernel(), Equals, "baz-linux")
	c.Check(model.Store(), Equals, "brand-store")
	// XXX: these are empty atm
	c.Check(model.AllowedModes(), HasLen, 0)
	c.Check(model.RequiredSnaps(), HasLen, 0)
}

const (
	modelErrPrefix = "assertion model: "
)

func (mods *modelSuite) TestDecodeInvalid(c *C) {
	encoded := strings.Replace(modelExample, "TSLINE", mods.tsLine, 1)

	invalidTests := []struct{ original, invalid, expectedErr string }{
		{"series: 16\n", "", `"series" header is mandatory`},
		{"series: 16\n", "series: \n", `"series" header should not be empty`},
		{"brand-id: brand-id1\n", "", `"brand-id" header is mandatory`},
		{"brand-id: brand-id1\n", "brand-id: \n", `"brand-id" header should not be empty`},
		{"brand-id: brand-id1\n", "brand-id: random\n", `authority-id and brand-id must match, model assertions are expected to be signed by the brand: "brand-id1" != "random"`},
		{"model: baz-3000\n", "", `"model" header is mandatory`},
		{"model: baz-3000\n", "model: \n", `"model" header should not be empty`},
		{"model: baz-3000\n", "model: baz/3000\n", `"model" primary key header cannot contain '/'`},
		{"core: core\n", "", `"core" header is mandatory`},
		{"core: core\n", "core: \n", `"core" header should not be empty`},
		{"architecture: amd64\n", "", `"architecture" header is mandatory`},
		{"architecture: amd64\n", "architecture: \n", `"architecture" header should not be empty`},
		{"gadget: brand-gadget\n", "", `"gadget" header is mandatory`},
		{"gadget: brand-gadget\n", "gadget: \n", `"gadget" header should not be empty`},
		{"kernel: baz-linux\n", "", `"kernel" header is mandatory`},
		{"kernel: baz-linux\n", "kernel: \n", `"kernel" header should not be empty`},
		{"store: brand-store\n", "", `"store" header is mandatory`},
		{"store: brand-store\n", "store: \n", `"store" header should not be empty`},
		{"class: fixed\n", "", `"class" header is mandatory`},
		{"class: fixed\n", "class: \n", `"class" header should not be empty`},
		{mods.tsLine, "", `"timestamp" header is mandatory`},
		{mods.tsLine, "timestamp: \n", `"timestamp" header should not be empty`},
		{mods.tsLine, "timestamp: 12:30\n", `"timestamp" header is not a RFC3339 date: .*`},
	}

	for _, test := range invalidTests {
		invalid := strings.Replace(encoded, test.original, test.invalid, 1)
		_, err := asserts.Decode([]byte(invalid))
		c.Check(err, ErrorMatches, modelErrPrefix+test.expectedErr)
	}
}

func (mods *modelSuite) TestModelCheck(c *C) {
	ex, err := asserts.Decode([]byte(strings.Replace(modelExample, "TSLINE", mods.tsLine, 1)))
	c.Assert(err, IsNil)

	storeDB, db := makeStoreAndCheckDB(c)
	brandDB := setup3rdPartySigning(c, "brand1", storeDB, db)

	headers := ex.Headers()
	headers["brand-id"] = brandDB.AuthorityID
	headers["timestamp"] = time.Now().Format(time.RFC3339)
	model, err := brandDB.Sign(asserts.ModelType, headers, nil, "")
	c.Assert(err, IsNil)

	err = db.Check(model)
	c.Assert(err, IsNil)
}

func (mods *modelSuite) TestModelCheckInconsistentTimestamp(c *C) {
	ex, err := asserts.Decode([]byte(strings.Replace(modelExample, "TSLINE", mods.tsLine, 1)))
	c.Assert(err, IsNil)

	storeDB, db := makeStoreAndCheckDB(c)
	brandDB := setup3rdPartySigning(c, "brand1", storeDB, db)

	headers := ex.Headers()
	headers["brand-id"] = brandDB.AuthorityID
	headers["timestamp"] = "2011-01-01T14:00:00Z"
	model, err := brandDB.Sign(asserts.ModelType, headers, nil, "")
	c.Assert(err, IsNil)

	err = db.Check(model)
	c.Assert(err, ErrorMatches, "model assertion timestamp outside of signing key validity")
}

type serialSuite struct {
	ts            time.Time
	tsLine        string
	deviceKey     asserts.PrivateKey
	encodedDevKey string
}

func (ss *serialSuite) SetUpSuite(c *C) {
	ss.ts = time.Now().Truncate(time.Second).UTC()
	ss.tsLine = "timestamp: " + ss.ts.Format(time.RFC3339) + "\n"

	ss.deviceKey = testPrivKey2
	encodedPubKey, err := asserts.EncodePublicKey(ss.deviceKey.PublicKey())
	c.Assert(err, IsNil)
	ss.encodedDevKey = string(encodedPubKey)
}

const serialExample = "type: serial\n" +
	"authority-id: canonical\n" +
	"brand-id: brand-id1\n" +
	"model: baz-3000\n" +
	"serial: 2700\n" +
	"device-key:\n    DEVICEKEY\n" +
	"device-key-sha3-384: KEYID\n" +
	"TSLINE" +
	"body-length: 2\n" +
	"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
	"HW" +
	"\n\n" +
	"AXNpZw=="

func (ss *serialSuite) TestDecodeOK(c *C) {
	encoded := strings.Replace(serialExample, "TSLINE", ss.tsLine, 1)
	encoded = strings.Replace(encoded, "DEVICEKEY", strings.Replace(ss.encodedDevKey, "\n", "\n    ", -1), 1)
	encoded = strings.Replace(encoded, "KEYID", ss.deviceKey.PublicKey().ID(), 1)
	a, err := asserts.Decode([]byte(encoded))
	c.Assert(err, IsNil)
	c.Check(a.Type(), Equals, asserts.SerialType)
	serial := a.(*asserts.Serial)
	c.Check(serial.AuthorityID(), Equals, "canonical")
	c.Check(serial.Timestamp(), Equals, ss.ts)
	c.Check(serial.BrandID(), Equals, "brand-id1")
	c.Check(serial.Model(), Equals, "baz-3000")
	c.Check(serial.Serial(), Equals, "2700")
	c.Check(serial.DeviceKey().ID(), Equals, ss.deviceKey.PublicKey().ID())
}

const (
	serialErrPrefix    = "assertion serial: "
	serialReqErrPrefix = "assertion serial-request: "
)

func (ss *serialSuite) TestDecodeInvalid(c *C) {
	encoded := strings.Replace(serialExample, "TSLINE", ss.tsLine, 1)

	invalidTests := []struct{ original, invalid, expectedErr string }{
		{"brand-id: brand-id1\n", "", `"brand-id" header is mandatory`},
		{"brand-id: brand-id1\n", "brand-id: \n", `"brand-id" header should not be empty`},
		{"model: baz-3000\n", "", `"model" header is mandatory`},
		{"model: baz-3000\n", "model: \n", `"model" header should not be empty`},
		{"serial: 2700\n", "", `"serial" header is mandatory`},
		{"serial: 2700\n", "serial: \n", `"serial" header should not be empty`},
		{ss.tsLine, "", `"timestamp" header is mandatory`},
		{ss.tsLine, "timestamp: \n", `"timestamp" header should not be empty`},
		{ss.tsLine, "timestamp: 12:30\n", `"timestamp" header is not a RFC3339 date: .*`},
		{"device-key:\n    DEVICEKEY\n", "", `"device-key" header is mandatory`},
		{"device-key:\n    DEVICEKEY\n", "device-key: \n", `"device-key" header should not be empty`},
		{"device-key:\n    DEVICEKEY\n", "device-key: $$$\n", `cannot decode public key: .*`},
	}

	for _, test := range invalidTests {
		invalid := strings.Replace(encoded, test.original, test.invalid, 1)
		invalid = strings.Replace(invalid, "DEVICEKEY", strings.Replace(ss.encodedDevKey, "\n", "\n    ", -1), 1)
		invalid = strings.Replace(invalid, "KEYID", ss.deviceKey.PublicKey().ID(), 1)
		_, err := asserts.Decode([]byte(invalid))
		c.Check(err, ErrorMatches, serialErrPrefix+test.expectedErr)
	}
}

func (ss *serialSuite) TestDecodeKeyIDMismatch(c *C) {
	invalid := strings.Replace(serialExample, "TSLINE", ss.tsLine, 1)
	invalid = strings.Replace(invalid, "DEVICEKEY", strings.Replace(ss.encodedDevKey, "\n", "\n    ", -1), 1)
	invalid = strings.Replace(invalid, "KEYID", "Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij", 1)

	_, err := asserts.Decode([]byte(invalid))
	c.Check(err, ErrorMatches, serialErrPrefix+"device key does not match provided key id")
}

func (ss *serialSuite) TestSerialRequestHappy(c *C) {
	sreq, err := asserts.SignWithoutAuthority(asserts.SerialRequestType,
		map[string]interface{}{
			"brand-id": "brand-id1",
			"model":    "baz-3000",
			// TODO add key hash header
			"device-key": ss.encodedDevKey,
			"request-id": "REQID",
		}, []byte("HW-DETAILS"), ss.deviceKey)
	c.Assert(err, IsNil)

	// roundtrip
	a, err := asserts.Decode(asserts.Encode(sreq))
	c.Assert(err, IsNil)

	sreq2, ok := a.(*asserts.SerialRequest)
	c.Assert(ok, Equals, true)

	// standalone signature check
	err = asserts.SignatureCheck(sreq2, sreq2.DeviceKey())
	c.Check(err, IsNil)

	c.Check(sreq2.BrandID(), Equals, "brand-id1")
	c.Check(sreq2.Model(), Equals, "baz-3000")
	c.Check(sreq2.RequestID(), Equals, "REQID")
}

func (ss *serialSuite) TestSerialRequestDecodeInvalid(c *C) {
	encoded := "type: serial-request\n" +
		"brand-id: brand-id1\n" +
		"model: baz-3000\n" +
		"device-key:\n    DEVICEKEY\n" +
		"request-id: REQID\n" +
		"body-length: 2\n" +
		"sign-key-sha3-384: " + ss.deviceKey.PublicKey().ID() + "\n\n" +
		"HW" +
		"\n\n" +
		"AXNpZw=="

	invalidTests := []struct{ original, invalid, expectedErr string }{
		{"brand-id: brand-id1\n", "", `"brand-id" header is mandatory`},
		{"brand-id: brand-id1\n", "brand-id: \n", `"brand-id" header should not be empty`},
		{"model: baz-3000\n", "", `"model" header is mandatory`},
		{"model: baz-3000\n", "model: \n", `"model" header should not be empty`},
		{"request-id: REQID\n", "", `"request-id" header is mandatory`},
		{"request-id: REQID\n", "request-id: \n", `"request-id" header should not be empty`},
		{"device-key:\n    DEVICEKEY\n", "", `"device-key" header is mandatory`},
		{"device-key:\n    DEVICEKEY\n", "device-key: \n", `"device-key" header should not be empty`},
		{"device-key:\n    DEVICEKEY\n", "device-key: $$$\n", `cannot decode public key: .*`},
	}

	for _, test := range invalidTests {
		invalid := strings.Replace(encoded, test.original, test.invalid, 1)
		invalid = strings.Replace(invalid, "DEVICEKEY", strings.Replace(ss.encodedDevKey, "\n", "\n    ", -1), 1)

		_, err := asserts.Decode([]byte(invalid))
		c.Check(err, ErrorMatches, serialReqErrPrefix+test.expectedErr)
	}
}

func (ss *serialSuite) TestSerialRequestDecodeKeyIDMismatch(c *C) {
	invalid := "type: serial-request\n" +
		"brand-id: brand-id1\n" +
		"model: baz-3000\n" +
		"device-key:\n    " + strings.Replace(ss.encodedDevKey, "\n", "\n    ", -1) + "\n" +
		"request-id: REQID\n" +
		"body-length: 2\n" +
		"sign-key-sha3-384: Jv8_JiHiIzJVcO9M55pPdqSDWUvuhfDIBJUS-3VW7F_idjix7Ffn5qMxB21ZQuij\n\n" +
		"HW" +
		"\n\n" +
		"AXNpZw=="

	_, err := asserts.Decode([]byte(invalid))
	c.Check(err, ErrorMatches, "assertion serial-request: device key does not match included signing key id")
}
