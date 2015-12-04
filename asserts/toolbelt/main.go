// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2015 Canonical Ltd
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

// Tool to create assertions for testing/playing purpose.
package main

import (
	"crypto"
	"fmt"
	"os"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"

	"github.com/ubuntu-core/snappy/asserts"
	"github.com/ubuntu-core/snappy/pkg/squashfs"
)

var parser = flags.NewParser(nil, flags.Default)

var db *asserts.Database

func main() {
	var err error
	cfg := &asserts.DatabaseConfig{
		Path: "snappy-asserts-toolbelt-db",
	}
	db, err = asserts.OpenDatabase(cfg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	parser.AddCommand("generate-key", "Generate key pair",
		"Generate key pair", &generateKey{})
	parser.AddCommand("account-key", "Make an account-key assertion",
		"Make an account-key assertion", &accountKey{})
	parser.AddCommand("snap-declaration", "Make a snap-declaration assertion",
		"Make a snap-declaration assertion", &snapDeclaration{})

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}
}

type generateKey struct {
	Positional struct {
		AuthorityID string `positional-arg-name:"authority-id"`
	} `positional-args:"yes"`
}

func (x *generateKey) Execute(args []string) error {
	authID := x.Positional.AuthorityID
	if authID == "" {
		return fmt.Errorf("missing authority-id")
	}

	fp, err := db.GenerateKey(authID)
	if err != nil {
		return err
	}
	fmt.Println(fp)
	return nil
}

func findAuthFingerprint(authID string) (string, error) {
	authPubKey, err := db.PublicKey(authID, "")
	if err != nil {
		return "", fmt.Errorf("failed to find signing key pair: %v", err)
	}
	return authPubKey.Fingerprint(), nil
}

type accountKey struct {
	Positional struct {
		AccountID   string `positional-arg-name:"account-id"`
		Years       uint   `positional-arg-name:"validity-years"`
		AuthorityID string `positional-arg-name:"authority-id"`
	} `positional-args:"yes"`
}

func (x *accountKey) Execute(args []string) error {
	accID := x.Positional.AccountID
	if accID == "" {
		return fmt.Errorf("missing account-id")
	}
	years := int(x.Positional.Years)
	if years == 0 {
		return fmt.Errorf("missing validity-years")
	}
	authID := x.Positional.AuthorityID
	if authID == "" {
		fmt.Fprintln(os.Stderr, "no authority-id: assume self-signed")
		authID = accID
	}

	authFingerprint, err := findAuthFingerprint(authID)
	if err != nil {
		return err
	}

	nowish := time.Now().Truncate(time.Hour).UTC()
	until := nowish.AddDate(years, 0, 0)
	pubKey, err := db.PublicKey(accID, "")
	if err != nil {
		return err
	}
	pubKeyBody, err := asserts.EncodePublicKey(pubKey)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"authority-id": authID,
		"account-id":   accID,
		"fingerprint":  pubKey.Fingerprint(),
		"since":        nowish.Format(time.RFC3339),
		"until":        until.Format(time.RFC3339),
	}
	accKey, err := db.Sign(asserts.AccountKeyType, headers, pubKeyBody, authFingerprint)
	if err != nil {
		return err
	}
	os.Stdout.Write(asserts.Encode(accKey))
	return nil
}

type snapDeclaration struct {
	Positional struct {
		AuthorityID string `positional-arg-name:"devel-id"`
		SnapFile    string `positional-arg-name:"squashfs-snap-file"`
	} `positional-args:"yes"`
}

func (x *snapDeclaration) Execute(args []string) error {
	authID := x.Positional.AuthorityID
	if authID == "" {
		return fmt.Errorf("missing devel/authority-id")
	}
	authFingerprint, err := findAuthFingerprint(authID)
	if err != nil {
		return err
	}

	snapFile := x.Positional.SnapFile
	if snapFile == "" {
		return fmt.Errorf("missing snap-file")
	}
	snap := squashfs.New(snapFile)
	nameParts := strings.SplitN(snap.Name(), "_", 2)
	snapID := nameParts[0] // XXX: cheat/guess
	size, hashDigest, err := snap.HashDigest(crypto.SHA256)
	if err != nil {
		return fmt.Errorf("failed to hash snap: %v", err)
	}
	formattedDigest, err := asserts.EncodeDigest(crypto.SHA256, hashDigest)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	headers := map[string]string{
		"authority-id": authID,
		"snap-id":      snapID,
		"snap-digest":  formattedDigest,
		"snap-size":    fmt.Sprintf("%d", size),
		"grade":        "devel",
		"timestamp":    now.Format(time.RFC3339),
	}
	snapDecl, err := db.Sign(asserts.SnapDeclarationType, headers, nil, authFingerprint)
	if err != nil {
		return err
	}
	os.Stdout.Write(asserts.Encode(snapDecl))
	return nil
}
