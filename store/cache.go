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

package store

import (
	"crypto"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/snapcore/snapd/osutil"
)

// downloadCache is the interface that a store download cache must provide
type downloadCache interface {
	// Get gets the given sha3 content and puts it into targetPath
	Get(sha3_384, targetPath string) error
	// Put adds a new file to the cache
	Put(sourcePath string) error
	// Lookup checks if the given sha3 content is available in the cache
	Lookup(sha3_384 string) bool
}

// nullCache is cache that does not cache
type nullCache struct{}

func (cm *nullCache) Get(sha3_384, targetPath string) error { return nil }
func (cm *nullCache) Put(sourcePath string) error           { return nil }
func (cm *nullCache) Lookup(sha3_384 string) bool           { return false }

// changesByReverseMtime sorts by the mtime of files
type changesByReverseMtime []os.FileInfo

func (s changesByReverseMtime) Len() int           { return len(s) }
func (s changesByReverseMtime) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s changesByReverseMtime) Less(i, j int) bool { return s[i].ModTime().After(s[j].ModTime()) }

// cacheManager implements a downloadCache via content based hard linking
type CacheManager struct {
	cacheDir string
	maxItems int
}

// NewCacheManager returns a new CacheManager with the given cacheDir
// and the given maximum amount of items. It works as outlined in:
//   https://forum.snapcraft.io/t/2374
func NewCacheManager(cacheDir string, maxItems int) *CacheManager {
	return &CacheManager{
		cacheDir: cacheDir,
		maxItems: maxItems,
	}
}

// Get gets the given sha3 content and puts it into targetPath
func (cm *CacheManager) Get(sha3_384, targetPath string) error {
	if !cm.Lookup(sha3_384) {
		return fmt.Errorf("cannot find %q in %q", sha3_384, cm.cacheDir)
	}
	if err := os.Link(cm.path(sha3_384), targetPath); err != nil {
		return err
	}
	return os.Chtimes(targetPath, time.Now(), time.Now())
}

// Put adds a new file to the cache
func (cm *CacheManager) Put(sourcePath string) error {
	// happens on e.g. `snap download` which runs as the user
	if !osutil.IsWritable(cm.cacheDir) {
		return nil
	}

	if err := os.MkdirAll(cm.cacheDir, 0700); err != nil && !os.IsExist(err) {
		return err
	}

	sha3_384, err := cm.digest(sourcePath)
	if err != nil {
		return err
	}
	if cm.Lookup(sha3_384) {
		return os.Chtimes(cm.path(sha3_384), time.Now(), time.Now())
	}

	if err := os.Link(sourcePath, cm.path(sha3_384)); err != nil {
		return err
	}
	return cm.cleanup()
}

// Lookup checks if the given sha3 content is available in the cache
func (cm *CacheManager) Lookup(sha3_384 string) bool {
	return osutil.FileExists(cm.path(sha3_384))
}

// Count returns the number of items in the cache
func (cm *CacheManager) Count() int {
	l, err := ioutil.ReadDir(cm.cacheDir)
	if err != nil {
		return 0
	}
	return len(l)
}

// digest returns the sha3 of the given path
func (cm *CacheManager) digest(path string) (string, error) {
	sha3_384_raw, _, err := osutil.FileDigest(path, crypto.SHA3_384)
	if err != nil {
		return "", err
	}
	sha3_384 := hex.EncodeToString(sha3_384_raw)
	return sha3_384, nil
}

// path returns the full path of the given content in the cache
func (cm *CacheManager) path(sha3_384 string) string {
	return filepath.Join(cm.cacheDir, sha3_384)
}

// cleanup ensures that only maxItems are stored in the cache
func (cm *CacheManager) cleanup() error {
	fil, err := ioutil.ReadDir(cm.cacheDir)
	if err != nil {
		return err
	}
	sort.Sort(changesByReverseMtime(fil))

	for i, fi := range fil {
		if i >= cm.maxItems {
			if err := os.Remove(cm.path(fi.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
