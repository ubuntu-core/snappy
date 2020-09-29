// -*- Mode: Go; indent-tabs-mode: t -*-
// +build !nosecboot

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

package secboot

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"

	sb "github.com/snapcore/secboot"

	"github.com/snapcore/snapd/osutil"
)

var (
	sbInitializeLUKS2Container       = sb.InitializeLUKS2Container
	sbAddRecoveryKeyToLUKS2Container = sb.AddRecoveryKeyToLUKS2Container
)

// RecoveryKey is a key used to unlock the encrypted partition when
// the encryption key can't be used, for example when unseal fails.
type RecoveryKey sb.RecoveryKey

func NewRecoveryKey() (RecoveryKey, error) {
	var key RecoveryKey
	// rand.Read() is protected against short reads
	_, err := rand.Read(key[:])
	// On return, n == len(b) if and only if err == nil
	return key, err
}

func RecoveryKeyFromFile(recoveryKeyFile string) (*RecoveryKey, error) {
	f, err := os.Open(recoveryKeyFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open recovery key: %v", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat recovery key: %v", err)
	}
	if st.Size() != int64(len(RecoveryKey{})) {
		return nil, fmt.Errorf("cannot read recovery key: unexpected size %v for the recovery key file", st.Size())
	}

	var rkey RecoveryKey
	if _, err := io.ReadFull(f, rkey[:]); err != nil {
		return nil, fmt.Errorf("cannot read recovery key: %v", err)
	}
	return &rkey, nil
}

// Save writes the recovery key in the location specified by filename.
func (key RecoveryKey) Save(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	return osutil.AtomicWriteFile(filename, key[:], 0600, 0)
}

// String provides a recovery key representation that can be read by
// secboot.ParseRecoveryKey() to contruct the recovery key from a string
func (key RecoveryKey) String() string {
	return sb.RecoveryKey(key).String()
}

// FormatEncryptedDevice initializes an encrypted volume on the block device
// given by node, setting the specified label. The key used to unlock the
// volume is provided using the key argument.
func FormatEncryptedDevice(key EncryptionKey, label, node string) error {
	return sbInitializeLUKS2Container(node, label, key[:])
}

// AddRecoveryKey adds a fallback recovery key rkey to the existing encrypted
// volume created with FormatEncryptedDevice on the block device given by node.
// The existing key to the encrypted volume is provided in the key argument.
func AddRecoveryKey(key EncryptionKey, rkey RecoveryKey, node string) error {
	return sbAddRecoveryKeyToLUKS2Container(node, key[:], sb.RecoveryKey(rkey))
}
