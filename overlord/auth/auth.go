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

package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"

	"gopkg.in/macaroon.v1"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/store"
)

// AuthState represents current authenticated users as tracked in state
type AuthState struct {
	LastID int          `json:"last-id"`
	Users  []UserState  `json:"users"`
	Device *DeviceState `json:"device,omitempty"`
}

// DeviceState represents the device's identity and store credentials
type DeviceState struct {
	Brand  string `json:"brand,omitempty"`
	Model  string `json:"model,omitempty"`
	Serial string `json:"serial,omitempty"`
	// XXX: SerialAssertion should eventually be retrieved from the
	// assertions DB, not persisted in state.
	SerialAssertion []byte   `json:"serial-assertion,omitempty"`
	StoreMacaroon   string   `json:"store-macaroon,omitempty"`
	StoreDischarges []string `json:"store-discharges,omitempty"`
}

// UserState represents an authenticated user
type UserState struct {
	ID              int      `json:"id"`
	Username        string   `json:"username,omitempty"`
	Macaroon        string   `json:"macaroon,omitempty"`
	Discharges      []string `json:"discharges,omitempty"`
	StoreMacaroon   string   `json:"store-macaroon,omitempty"`
	StoreDischarges []string `json:"store-discharges,omitempty"`
}

// NewUser tracks a new authenticated user and saves its details in the state
func NewUser(st *state.State, username, macaroon string, discharges []string) (*UserState, error) {
	var authStateData AuthState

	err := st.Get("auth", &authStateData)
	if err == state.ErrNoState {
		authStateData = AuthState{}
	} else if err != nil {
		return nil, err
	}

	sort.Strings(discharges)
	authStateData.LastID++
	authenticatedUser := UserState{
		ID:              authStateData.LastID,
		Username:        username,
		Macaroon:        macaroon,
		Discharges:      discharges,
		StoreMacaroon:   macaroon,
		StoreDischarges: discharges,
	}
	authStateData.Users = append(authStateData.Users, authenticatedUser)

	st.Set("auth", authStateData)

	return &authenticatedUser, nil
}

// RemoveUser removes a user from the state given its ID
func RemoveUser(st *state.State, userID int) error {
	var authStateData AuthState

	err := st.Get("auth", &authStateData)
	if err != nil {
		return err
	}

	for i := range authStateData.Users {
		if authStateData.Users[i].ID == userID {
			// delete without preserving order
			n := len(authStateData.Users) - 1
			authStateData.Users[i] = authStateData.Users[n]
			authStateData.Users[n] = UserState{}
			authStateData.Users = authStateData.Users[:n]
			st.Set("auth", authStateData)
			return nil
		}
	}

	return fmt.Errorf("invalid user")
}

// User returns a user from the state given its ID
func User(st *state.State, id int) (*UserState, error) {
	var authStateData AuthState

	err := st.Get("auth", &authStateData)
	if err != nil {
		return nil, err
	}

	for _, user := range authStateData.Users {
		if user.ID == id {
			return &user, nil
		}
	}
	return nil, fmt.Errorf("invalid user")
}

// SetDeviceIdentity sets the device's identity as used for communication with the store.
// XXX: Should eventually require the serial assertion to be the
// assertions DB rather than taking it directly, but that requires
// significant work.
// XXX: Doesn't check that the serial assertion is trustworthy, just
// that it matches the purported identity. But the assertion is only
// used to establish a store session, and the store performs the
// requisite checks.
func SetDeviceIdentity(st *state.State, brand string, model string, serial string, serialAssertionEncoded []byte) error {
	var authStateData AuthState

	// Verify that the serial assertion matches the brand, model and
	// serial.
	assert, err := asserts.Decode(serialAssertionEncoded)
	if err != nil {
		return err
	}
	if assert.Type() != asserts.SerialType {
		return fmt.Errorf("serial assertion is actually %s", assert.Type())
	}
	serialAssertion := assert.(*asserts.Serial)
	if brand != serialAssertion.BrandID() || model != serialAssertion.Model() || serial != serialAssertion.Serial() {
		return fmt.Errorf("serial assertion doesn't match purported identity")
	}

	err = st.Get("auth", &authStateData)
	if err == state.ErrNoState {
		authStateData = AuthState{}
	} else if err != nil {
		return err
	}

	if authStateData.Device == nil {
		authStateData.Device = &DeviceState{}
	}

	authStateData.Device.Brand = brand
	authStateData.Device.Model = model
	authStateData.Device.Serial = serial
	authStateData.Device.SerialAssertion = serialAssertionEncoded
	st.Set("auth", authStateData)

	return nil
}

// SetDeviceStoreMacaroon sets the credentials used to authenticate to the store as the device.
func SetDeviceStoreMacaroon(st *state.State, macaroon string, discharges []string) error {
	var authStateData AuthState

	err := st.Get("auth", &authStateData)
	if err == state.ErrNoState {
		authStateData = AuthState{}
	} else if err != nil {
		return err
	}

	if authStateData.Device == nil {
		authStateData.Device = &DeviceState{}
	}

	sort.Strings(discharges)
	authStateData.Device.StoreMacaroon = macaroon
	authStateData.Device.StoreDischarges = discharges
	st.Set("auth", authStateData)

	return nil
}

// Device returns the device details from the state, or nil if no device state exists.
func Device(st *state.State) (*DeviceState, error) {
	var authStateData AuthState

	err := st.Get("auth", &authStateData)
	if err == state.ErrNoState {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return authStateData.Device, nil
}

var ErrInvalidAuth = fmt.Errorf("invalid authentication")

// CheckMacaroon returns the UserState for the given macaroon/discharges credentials
func CheckMacaroon(st *state.State, macaroon string, discharges []string) (*UserState, error) {
	var authStateData AuthState
	err := st.Get("auth", &authStateData)
	if err != nil {
		return nil, ErrInvalidAuth
	}

NextUser:
	for _, user := range authStateData.Users {
		if user.Macaroon != macaroon {
			continue
		}
		if len(user.Discharges) != len(discharges) {
			continue
		}
		// sort discharges (stored users' discharges are already sorted)
		sort.Strings(discharges)
		for i, d := range user.Discharges {
			if d != discharges[i] {
				continue NextUser
			}
		}
		return &user, nil
	}
	return nil, ErrInvalidAuth
}

// Authenticator returns a store.Authenticator which adds the user and
// device credentials to the request if either exist, otherwise nil.
func Authenticator(st *state.State, userID int) (store.Authenticator, error) {
	var us *UserState
	var err error
	if userID != 0 {
		us, err = User(st, userID)
		if err != nil {
			return nil, err
		}
	}
	var ds *DeviceState
	ds, err = Device(st)
	if err != nil {
		return nil, err
	}

	// Collect any available user and device credentials.
	var userMacaroon, deviceMacaroon string
	var userDischarges, deviceDischarges []string
	if us != nil {
		userMacaroon = us.StoreMacaroon
		userDischarges = us.StoreDischarges
	}
	if ds != nil {
		deviceMacaroon = ds.StoreMacaroon
		deviceDischarges = ds.StoreDischarges
	}
	if userMacaroon != "" || deviceMacaroon != "" {
		return newMacaroonAuthenticator(userMacaroon, userDischarges, deviceMacaroon, deviceDischarges), nil
	}
	return nil, nil
}

// MacaroonAuthenticator is a store authenticator based on macaroons
type MacaroonAuthenticator struct {
	UserMacaroon     string
	UserDischarges   []string
	DeviceMacaroon   string
	DeviceDischarges []string
}

func newMacaroonAuthenticator(userMacaroon string, userDischarges []string, deviceMacaroon string, deviceDischarges []string) *MacaroonAuthenticator {
	return &MacaroonAuthenticator{
		UserMacaroon:     userMacaroon,
		UserDischarges:   userDischarges,
		DeviceMacaroon:   deviceMacaroon,
		DeviceDischarges: deviceDischarges,
	}
}

// MacaroonSerialize returns a store-compatible serialized representation of the given macaroon
func MacaroonSerialize(m *macaroon.Macaroon) (string, error) {
	marshalled, err := m.MarshalBinary()
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(marshalled)
	return encoded, nil
}

// MacaroonDeserialize returns a deserialized macaroon from a given store-compatible serialization
func MacaroonDeserialize(serializedMacaroon string) (*macaroon.Macaroon, error) {
	var m macaroon.Macaroon
	decoded, err := base64.RawURLEncoding.DecodeString(serializedMacaroon)
	if err != nil {
		return nil, err
	}
	err = m.UnmarshalBinary(decoded)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// LoginCaveatID returns the 3rd party caveat from the macaroon to be discharged by Ubuntuone
func LoginCaveatID(m *macaroon.Macaroon) (string, error) {
	caveatID := ""
	for _, caveat := range m.Caveats() {
		if caveat.Location == store.UbuntuoneLocation {
			caveatID = caveat.Id
			break
		}
	}
	if caveatID == "" {
		return "", fmt.Errorf("missing login caveat")
	}
	return caveatID, nil
}

func serializeMacaroonAuthorization(macaroon string, discharges []string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `Macaroon root="%s"`, macaroon)
	for _, discharge := range discharges {
		fmt.Fprintf(&buf, `, discharge="%s"`, discharge)
	}
	return buf.String()
}

// Authenticate will add the store expected Authorization and X-Device-Authorization headers for macaroons
func (ma *MacaroonAuthenticator) Authenticate(r *http.Request) {
	if ma.UserMacaroon != "" {
		r.Header.Set("Authorization", serializeMacaroonAuthorization(ma.UserMacaroon, ma.UserDischarges))
	}
	if ma.DeviceMacaroon != "" {
		r.Header.Set("X-Device-Authorization", serializeMacaroonAuthorization(ma.DeviceMacaroon, ma.DeviceDischarges))
	}
}
