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

// Package devicestate implements the manager and state aspects responsible
// for the device identity and policies.
package devicestate

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/overlord/assertstate"
	"github.com/snapcore/snapd/overlord/auth"
	"github.com/snapcore/snapd/overlord/boot"
	"github.com/snapcore/snapd/overlord/configstate"
	"github.com/snapcore/snapd/overlord/hookstate"
	"github.com/snapcore/snapd/overlord/snapstate"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/partition"
	"github.com/snapcore/snapd/release"
)

// DeviceManager is responsible for managing the device identity and device
// policies.
type DeviceManager struct {
	state      *state.State
	keypairMgr asserts.KeypairManager
	runner     *state.TaskRunner
}

// Manager returns a new device manager.
func Manager(s *state.State, hookManager *hookstate.HookManager) (*DeviceManager, error) {
	runner := state.NewTaskRunner(s)

	keypairMgr, err := asserts.OpenFSKeypairManager(dirs.SnapDeviceDir)
	if err != nil {
		return nil, err

	}

	m := &DeviceManager{state: s, keypairMgr: keypairMgr, runner: runner}

	hookManager.Register(regexp.MustCompile("^prepare-device$"), newPrepareDeviceHandler)

	runner.AddHandler("generate-device-key", m.doGenerateDeviceKey, nil)
	runner.AddHandler("request-serial", m.doRequestSerial, nil)

	return m, nil
}

type prepareDeviceHandler struct{}

func newPrepareDeviceHandler(context *hookstate.Context) hookstate.Handler {
	return prepareDeviceHandler{}
}

func (h prepareDeviceHandler) Before() error {
	return nil
}

func (h prepareDeviceHandler) Done() error {
	return nil
}

func (h prepareDeviceHandler) Error(err error) error {
	return nil
}

func (m *DeviceManager) ensureOperational() error {
	m.state.Lock()
	defer m.state.Unlock()

	device, err := auth.Device(m.state)
	if err != nil {
		return err
	}

	if device.Serial != "" {
		// serial is set, we are all set
		return nil
	}

	if device.Brand == "" || device.Model == "" {
		// need first-boot, loading of model assertion info
		if release.OnClassic {
			// TODO: are we going to have model assertions on classic or need will need to cheat here?
			return nil
		}
		// cannot proceed yet, once first boot is done these will be set
		// and we can pick up from there
		return nil
	}

	for _, chg := range m.state.Changes() {
		if chg.Kind() == "become-operational" && !chg.Status().Ready() {
			// change already in motion
			return nil
		}
	}

	if serialRequestURL == "" {
		// cannot do anything actually
		return nil
	}

	gadgetInfo, err := snapstate.GadgetInfo(m.state)
	if err == state.ErrNoState {
		// no gadget installed yet, cannot proceed
		return nil
	}
	if err != nil {
		return err
	}

	// XXX: some of these will need to be split and use hooks
	// retries might need to embrace more than one "task" then,
	// need to be careful

	tasks := []*state.Task{}

	var prepareDevice *state.Task
	if gadgetInfo.Hooks["prepare-device"] != nil {
		summary := i18n.G("Run prepare-device hook")
		hooksup := &hookstate.HookSetup{
			Snap: gadgetInfo.Name(),
			Hook: "prepare-device",
		}
		prepareDevice = hookstate.HookTask(m.state, summary, hooksup, nil)
		tasks = append(tasks, prepareDevice)
	}

	genKey := m.state.NewTask("generate-device-key", i18n.G("Generate device key"))
	if prepareDevice != nil {
		genKey.WaitFor(prepareDevice)
	}
	tasks = append(tasks, genKey)
	requestSerial := m.state.NewTask("request-serial", i18n.G("Request device serial"))
	requestSerial.WaitFor(genKey)
	tasks = append(tasks, requestSerial)

	chg := m.state.NewChange("become-operational", i18n.G("Initialize device"))
	chg.AddAll(state.NewTaskSet(tasks...))

	return nil
}

// ensureSnaps makes sure that the snaps from seed.yaml get installed
// with the matching assertions
func (m *DeviceManager) ensureSeedYaml() error {
	m.state.Lock()
	defer m.state.Unlock()

	// FIXME: enable on classic?
	//
	// Disable seed.yaml on classic for now. In the long run we want
	// classic to have a seed parsing as well so that we can install
	// snaps in a classic environment (LP: #1609903). However right
	// now it is under heavy development so until the dust
	// settles we disable it.
	if release.OnClassic {
		return nil
	}

	all, err := snapstate.All(m.state)
	if err != nil {
		return err
	}
	if len(all) > 0 {
		return nil
	}

	for _, chg := range m.state.Changes() {
		if chg.Kind() == "seed" && !chg.Status().Ready() {
			// change already in motion
			return nil
		}
	}

	if err := boot.PopulateStateFromSeed(m.state); err != nil {
		return err
	}
	m.state.EnsureBefore(0)

	return nil
}

func (m *DeviceManager) ensureBootOk() error {
	m.state.Lock()
	defer m.state.Unlock()

	if release.OnClassic {
		logger.Debugf("Ignoring 'booted' on classic")
		return nil
	}

	bootloader, err := partition.FindBootloader()
	if err != nil {
		return fmt.Errorf(i18n.G("cannot mark boot successful: %s"), err)
	}

	if err := partition.MarkBootSuccessful(bootloader); err != nil {
		return err
	}

	return snapstate.UpdateRevisions(m.state)
}

// Ensure implements StateManager.Ensure.
func (m *DeviceManager) Ensure() error {
	if err := m.ensureSeedYaml(); err != nil {
		return err
	}
	if err := m.ensureOperational(); err != nil {
		return err
	}
	if err := m.ensureBootOk(); err != nil {
		return err
	}

	m.runner.Ensure()
	return nil
}

// Wait implements StateManager.Wait.
func (m *DeviceManager) Wait() {
	m.runner.Wait()
}

// Stop implements StateManager.Stop.
func (m *DeviceManager) Stop() {
	m.runner.Stop()
}

func useStaging() bool {
	return os.Getenv("SNAPPY_USE_STAGING_STORE") == "1"
}

func deviceAPIBaseURL() string {
	if useStaging() {
		return "https://myapps.developer.staging.ubuntu.com/identity/api/v1/"
	}
	return "https://myapps.developer.ubuntu.com/identity/api/v1/"
}

var (
	keyLength        = 4096
	retryInterval    = 60 * time.Second
	deviceAPIBase    = deviceAPIBaseURL()
	requestIDURL     = deviceAPIBase + "request-id"
	serialRequestURL = deviceAPIBase + "devices"
)

func (m *DeviceManager) doGenerateDeviceKey(t *state.Task, _ *tomb.Tomb) error {
	st := t.State()
	st.Lock()
	defer st.Unlock()

	device, err := auth.Device(st)
	if err != nil {
		return err
	}

	if device.KeyID != "" {
		// nothing to do
		return nil
	}

	keyPair, err := rsa.GenerateKey(rand.Reader, keyLength)
	if err != nil {
		return fmt.Errorf("cannot generate device key pair: %v", err)
	}

	privKey := asserts.RSAPrivateKey(keyPair)
	err = m.keypairMgr.Put(privKey)
	if err != nil {
		return fmt.Errorf("cannot store device key pair: %v", err)
	}

	device.KeyID = privKey.PublicKey().ID()
	auth.SetDevice(st, device)
	t.SetStatus(state.DoneStatus)
	return nil
}

func (m *DeviceManager) keyPair() (asserts.PrivateKey, error) {
	device, err := auth.Device(m.state)
	if err != nil {
		return nil, err
	}

	if device.KeyID == "" {
		return nil, state.ErrNoState
	}

	privKey, err := m.keypairMgr.Get(device.KeyID)
	if err != nil {
		return nil, fmt.Errorf("cannot read device key pair: %v", err)
	}
	return privKey, nil
}

type serialSetup struct {
	SerialRequest string `json:"serial-request"`
	Serial        string `json:"serial"`
}

type requestIDResp struct {
	RequestID string `json:"request-id"`
}

func retryErr(t *state.Task, reason string, a ...interface{}) error {
	t.State().Lock()
	defer t.State().Unlock()
	t.Errorf(reason, a...)
	return &state.Retry{After: retryInterval}
}

func prepareSerialRequest(t *state.Task, privKey asserts.PrivateKey, device *auth.DeviceState, client *http.Client, cfg *serialRequestConfig) (string, error) {
	st := t.State()
	st.Unlock()
	defer st.Lock()

	req, err := http.NewRequest("POST", cfg.requestIDURL, nil)
	if err != nil {
		return "", fmt.Errorf("internal error: cannot create request-id request %q", cfg.requestIDURL)
	}
	cfg.applyHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", retryErr(t, "cannot retrieve request-id for making a request for a serial: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", retryErr(t, "cannot retrieve request-id for making a request for a serial: unexpected status %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	var requestID requestIDResp
	err = dec.Decode(&requestID)
	if err != nil { // assume broken i/o
		return "", retryErr(t, "cannot read response with request-id for making a request for a serial: %v", err)
	}

	encodedPubKey, err := asserts.EncodePublicKey(privKey.PublicKey())
	if err != nil {
		return "", fmt.Errorf("internal error: cannot encode device public key: %v", err)

	}

	headers := map[string]interface{}{
		"brand-id":   device.Brand,
		"model":      device.Model,
		"request-id": requestID.RequestID,
		"device-key": string(encodedPubKey),
	}
	if cfg.proposedSerial != "" {
		headers["serial"] = cfg.proposedSerial
	}

	serialReq, err := asserts.SignWithoutAuthority(asserts.SerialRequestType, headers, cfg.body, privKey)
	if err != nil {
		return "", err
	}

	return string(asserts.Encode(serialReq)), nil
}

var errPoll = errors.New("serial-request accepted, poll later")

func submitSerialRequest(t *state.Task, serialRequest string, client *http.Client, cfg *serialRequestConfig) (*asserts.Serial, error) {
	st := t.State()
	st.Unlock()
	defer st.Lock()

	req, err := http.NewRequest("POST", cfg.serialRequestURL, bytes.NewBufferString(serialRequest))
	if err != nil {
		return nil, fmt.Errorf("internal error: cannot create serial-request request %q", cfg.serialRequestURL)
	}
	cfg.applyHeaders(req)
	req.Header.Set("Content-Type", asserts.MediaType)

	resp, err := client.Do(req)
	if err != nil {
		return nil, retryErr(t, "cannot deliver device serial request: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200, 201:
	case 202:
		return nil, errPoll
	default:
		return nil, retryErr(t, "cannot deliver device serial request: unexpected status %d", resp.StatusCode)
	}

	// decode body with serial assertion
	dec := asserts.NewDecoder(resp.Body)
	got, err := dec.Decode()
	if err != nil { // assume broken i/o
		return nil, retryErr(t, "cannot read response to request for a serial: %v", err)
	}

	serial, ok := got.(*asserts.Serial)
	if !ok {
		return nil, fmt.Errorf("cannot use device serial assertion of type %q", got.Type().Name)
	}

	return serial, nil
}

func getSerial(t *state.Task, privKey asserts.PrivateKey, device *auth.DeviceState, cfg *serialRequestConfig) (*asserts.Serial, error) {
	var serialSup serialSetup
	err := t.Get("serial-setup", &serialSup)
	if err != nil && err != state.ErrNoState {
		return nil, err
	}

	if serialSup.Serial != "" {
		// we got a serial, just haven't managed to save its info yet
		a, err := asserts.Decode([]byte(serialSup.Serial))
		if err != nil {
			return nil, fmt.Errorf("internal error: cannot decode previously saved serial: %v", err)
		}
		return a.(*asserts.Serial), nil
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// NB: until we get at least an Accepted (202) we need to
	// retry from scratch creating a new request-id because the
	// previous one used could have expired

	if serialSup.SerialRequest == "" {
		serialRequest, err := prepareSerialRequest(t, privKey, device, client, cfg)
		if err != nil { // errors & retries
			return nil, err
		}

		serialSup.SerialRequest = serialRequest
	}

	serial, err := submitSerialRequest(t, serialSup.SerialRequest, client, cfg)
	if err == errPoll {
		// we can/should reuse the serial-request
		t.Set("serial-setup", serialSup)
		return nil, errPoll
	}
	if err != nil { // errors & retries
		return nil, err
	}

	keyID := privKey.PublicKey().ID()
	if serial.BrandID() != device.Brand || serial.Model() != device.Model || serial.DeviceKey().ID() != keyID {
		return nil, fmt.Errorf("obtained serial assertion does not match provided device identity information (brand, model, key id): %s / %s / %s != %s / %s / %s", serial.BrandID(), serial.Model(), serial.DeviceKey().ID(), device.Brand, device.Model, keyID)
	}

	serialSup.Serial = string(asserts.Encode(serial))
	t.Set("serial-setup", serialSup)

	if repeatRequestSerial == "after-got-serial" {
		// For testing purposes, ensure a crash in this state works.
		return nil, &state.Retry{}
	}

	return serial, nil
}

type serialRequestConfig struct {
	requestIDURL     string
	serialRequestURL string
	headers          map[string]string
	proposedSerial   string
	body             []byte
}

func (cfg *serialRequestConfig) applyHeaders(req *http.Request) {
	for k, v := range cfg.headers {
		req.Header.Set(k, v)
	}
}

func getSerialRequestConfig(t *state.Task) (*serialRequestConfig, error) {
	gadgetInfo, err := snapstate.GadgetInfo(t.State())
	if err != nil {
		return nil, fmt.Errorf("cannot find gadget snap and its name: %v", err)
	}
	gadgetName := gadgetInfo.Name()

	tr := configstate.NewTransaction(t.State())
	var svcURL string
	err = tr.GetMaybe(gadgetName, "device-service.url", &svcURL)
	if err != nil {
		return nil, err
	}

	if svcURL != "" {
		baseURL, err := url.Parse(svcURL)
		if err != nil {
			return nil, fmt.Errorf("cannot parse device registration base URL %q: %v", svcURL, err)
		}

		var headers map[string]string
		err = tr.GetMaybe(gadgetName, "device-service.headers", &headers)
		if err != nil {
			return nil, err
		}

		cfg := serialRequestConfig{
			headers: headers,
		}

		reqIDURL, err := baseURL.Parse("request-id")
		if err != nil {
			return nil, fmt.Errorf("cannot build /request-id URL from %v: %v", baseURL, err)
		}
		cfg.requestIDURL = reqIDURL.String()

		var bodyStr string
		err = tr.GetMaybe(gadgetName, "registration.body", &bodyStr)
		if err != nil {
			return nil, err
		}

		cfg.body = []byte(bodyStr)

		serialURL, err := baseURL.Parse("serial")
		if err != nil {
			return nil, fmt.Errorf("cannot build /serial URL from %v: %v", baseURL, err)
		}
		cfg.serialRequestURL = serialURL.String()

		var proposedSerial string
		err = tr.GetMaybe(gadgetName, "registration.proposed-serial", &proposedSerial)
		if err != nil {
			return nil, err
		}
		cfg.proposedSerial = proposedSerial

		return &cfg, nil
	}

	return &serialRequestConfig{
		requestIDURL:     requestIDURL,
		serialRequestURL: serialRequestURL,
	}, nil
}

func (m *DeviceManager) doRequestSerial(t *state.Task, _ *tomb.Tomb) error {
	st := t.State()
	st.Lock()
	defer st.Unlock()

	cfg, err := getSerialRequestConfig(t)
	if err != nil {
		return err
	}

	device, err := auth.Device(st)
	if err != nil {
		return err
	}

	privKey, err := m.keyPair()
	if err == state.ErrNoState {
		return fmt.Errorf("internal error: cannot find device key pair")
	}
	if err != nil {
		return err
	}

	// make this idempotent, look if we have already a serial assertion
	// for privKey
	serials, err := assertstate.DB(st).FindMany(asserts.SerialType, map[string]string{
		"brand-id":            device.Brand,
		"model":               device.Model,
		"device-key-sha3-384": privKey.PublicKey().ID(),
	})

	if len(serials) == 1 {
		// means we saved the assertion but didn't get to the end of the task
		device.Serial = serials[0].(*asserts.Serial).Serial()
		auth.SetDevice(st, device)
		t.SetStatus(state.DoneStatus)
		return nil
	}
	if len(serials) > 1 {
		return fmt.Errorf("internal error: multiple serial assertions for the same device key")
	}

	serial, err := getSerial(t, privKey, device, cfg)
	if err == errPoll {
		t.Logf("Will poll for device serial assertion in 60 seconds")
		return &state.Retry{After: retryInterval}
	}
	if err != nil { // errors & retries
		return err
	}

	sto := snapstate.Store(st)
	// try to fetch the signing key of the serial
	st.Unlock()
	a, errAcctKey := sto.Assertion(asserts.AccountKeyType, []string{serial.SignKeyID()}, nil)
	st.Lock()
	if errAcctKey == nil {
		err := assertstate.Add(st, a)
		if err != nil {
			if _, ok := err.(*asserts.RevisionError); !ok {
				return err
			}
		}
	}

	// add the serial assertion to the system assertion db
	err = assertstate.Add(st, serial)
	if err != nil {
		// if we had failed to fetch the signing key, retry in a bit
		if errAcctKey != nil {
			t.Errorf("cannot fetch signing key for the serial: %v", errAcctKey)
			return &state.Retry{After: retryInterval}
		}
		return err
	}

	if repeatRequestSerial == "after-add-serial" {
		// For testing purposes, ensure a crash in this state works.
		return &state.Retry{}
	}

	device.Serial = serial.Serial()
	auth.SetDevice(st, device)

	t.SetStatus(state.DoneStatus)
	return nil
}

var repeatRequestSerial string

// implementing auth.DeviceAssertions
// sanity check
var _ auth.DeviceAssertions = (*DeviceManager)(nil)

// Model returns the device model assertion.
func (m *DeviceManager) Model() (*asserts.Model, error) {
	m.state.Lock()
	defer m.state.Unlock()

	return Model(m.state)
}

// Serial returns the device serial assertion.
func (m *DeviceManager) Serial() (*asserts.Serial, error) {
	m.state.Lock()
	defer m.state.Unlock()

	return Serial(m.state)
}

// DeviceSessionRequest produces a device-session-request with the given nonce, it also returns the device serial assertion.
func (m *DeviceManager) DeviceSessionRequest(nonce string) (*asserts.DeviceSessionRequest, *asserts.Serial, error) {
	m.state.Lock()
	defer m.state.Unlock()

	serial, err := Serial(m.state)
	if err != nil {
		return nil, nil, err
	}

	privKey, err := m.keyPair()
	if err != nil {
		return nil, nil, err
	}

	a, err := asserts.SignWithoutAuthority(asserts.DeviceSessionRequestType, map[string]interface{}{
		"brand-id":  serial.BrandID(),
		"model":     serial.Model(),
		"serial":    serial.Serial(),
		"nonce":     nonce,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}, nil, privKey)
	if err != nil {
		return nil, nil, err
	}

	return a.(*asserts.DeviceSessionRequest), serial, err

}

// Model returns the device model assertion.
func Model(st *state.State) (*asserts.Model, error) {
	device, err := auth.Device(st)
	if err != nil {
		return nil, err
	}

	if device.Brand == "" || device.Model == "" {
		return nil, state.ErrNoState
	}

	a, err := assertstate.DB(st).Find(asserts.ModelType, map[string]string{
		"series":   release.Series,
		"brand-id": device.Brand,
		"model":    device.Model,
	})
	if err == asserts.ErrNotFound {
		return nil, state.ErrNoState
	}
	if err != nil {
		return nil, err
	}

	return a.(*asserts.Model), nil
}

// Serial returns the device serial assertion.
func Serial(st *state.State) (*asserts.Serial, error) {
	device, err := auth.Device(st)
	if err != nil {
		return nil, err
	}

	if device.Serial == "" {
		return nil, state.ErrNoState
	}

	a, err := assertstate.DB(st).Find(asserts.SerialType, map[string]string{
		"brand-id": device.Brand,
		"model":    device.Model,
		"serial":   device.Serial,
	})
	if err == asserts.ErrNotFound {
		return nil, state.ErrNoState
	}
	if err != nil {
		return nil, err
	}

	return a.(*asserts.Serial), nil
}
