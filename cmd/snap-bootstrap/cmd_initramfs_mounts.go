// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019 Canonical Ltd
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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/boot"
	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/sysconfig"
	"github.com/snapcore/snapd/timings"
)

func init() {
	const (
		short = "Generate initramfs mount tuples"
		long  = "Generate mount tuples for the initramfs until nothing more can be done"
	)

	addCommandBuilder(func(parser *flags.Parser) {
		if _, err := parser.AddCommand("initramfs-mounts", short, long, &cmdInitramfsMounts{}); err != nil {
			panic(err)
		}
	})

	snap.SanitizePlugsSlots = func(*snap.Info) {}
}

type cmdInitramfsMounts struct{}

func (c *cmdInitramfsMounts) Execute(args []string) error {
	return generateInitramfsMounts()
}

var (
	// Stdout - can be overridden in tests
	stdout io.Writer = os.Stdout
)

var (
	osutilIsMounted = osutil.IsMounted
)

// TODO:UC20: re-write the install mode function to use the boot pkg functions
// and get rid of this function
func recoverySystemEssentialSnaps(seedDir, recoverySystem string, essentialTypes []snap.Type) ([]*seed.Snap, error) {
	systemSeed, err := seed.Open(seedDir, recoverySystem)
	if err != nil {
		return nil, err
	}

	seed20, ok := systemSeed.(seed.EssentialMetaLoaderSeed)
	if !ok {
		return nil, fmt.Errorf("internal error: UC20 seed must implement EssentialMetaLoaderSeed")
	}

	// load assertions into a temporary database
	if err := systemSeed.LoadAssertions(nil, nil); err != nil {
		return nil, err
	}

	// load and verify metadata only for the relevant essential snaps
	perf := timings.New(nil)
	if err := seed20.LoadEssentialMeta(essentialTypes, perf); err != nil {
		return nil, err
	}

	return seed20.EssentialSnaps(), nil
}

// generateMountsMode* is called multiple times from initramfs until it
// no longer generates more mount points and just returns an empty output.
func generateMountsModeInstall(recoverySystem string) error {
	seedDir := filepath.Join(dirs.RunMnt, "ubuntu-seed")

	// 1. always ensure seed partition is mounted
	isMounted, err := osutilIsMounted(seedDir)
	if err != nil {
		return err
	}
	if !isMounted {
		fmt.Fprintf(stdout, "/dev/disk/by-label/ubuntu-seed %s\n", seedDir)
		return nil
	}

	// 2. (auto) select recovery system for now
	isBaseMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "base"))
	if err != nil {
		return err
	}
	isKernelMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "kernel"))
	if err != nil {
		return err
	}
	isSnapdMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "snapd"))
	if err != nil {
		return err
	}
	if !isBaseMounted || !isKernelMounted || !isSnapdMounted {
		// load the recovery system and generate mounts for kernel/base
		// and snapd
		var whichTypes []snap.Type
		if !isBaseMounted {
			whichTypes = append(whichTypes, snap.TypeBase)
		}
		if !isKernelMounted {
			whichTypes = append(whichTypes, snap.TypeKernel)
		}
		if !isSnapdMounted {
			whichTypes = append(whichTypes, snap.TypeSnapd)
		}
		// TODO:UC20: use more generalized version of
		//            boot.InitramfsRunModeSnapsToMount here to fully move
		//            recoverySystemEssentialSnaps to boot pkg
		essSnaps, err := recoverySystemEssentialSnaps(seedDir, recoverySystem, whichTypes)
		if err != nil {
			return fmt.Errorf("cannot load metadata and verify essential bootstrap snaps %v: %v", whichTypes, err)
		}

		// TODO:UC20: do we need more cross checks here?
		for _, essentialSnap := range essSnaps {
			switch essentialSnap.EssentialType {
			case snap.TypeBase:
				fmt.Fprintf(stdout, "%s %s\n", essentialSnap.Path, filepath.Join(dirs.RunMnt, "base"))
			case snap.TypeKernel:
				// TODO:UC20: we need to cross-check the kernel path with snapd_recovery_kernel used by grub
				fmt.Fprintf(stdout, "%s %s\n", essentialSnap.Path, filepath.Join(dirs.RunMnt, "kernel"))
			case snap.TypeSnapd:
				fmt.Fprintf(stdout, "%s %s\n", essentialSnap.Path, filepath.Join(dirs.RunMnt, "snapd"))
			}
		}
	}

	// 3. mount "ubuntu-data" on a tmpfs
	isMounted, err = osutilIsMounted(filepath.Join(dirs.RunMnt, "ubuntu-data"))
	if err != nil {
		return err
	}
	if !isMounted {
		// TODO:UC20: is there a better way?
		fmt.Fprintf(stdout, "--type=tmpfs tmpfs /run/mnt/ubuntu-data\n")
		return nil
	}

	// 4. final step: write $(ubuntu_data)/var/lib/snapd/modeenv - this
	//    is the tmpfs we just created above
	modeEnv := &boot.Modeenv{
		Mode:           "install",
		RecoverySystem: recoverySystem,
	}
	if err := modeEnv.Write(filepath.Join(dirs.RunMnt, "ubuntu-data", "system-data")); err != nil {
		return err
	}
	// and disable cloud-init in install mode
	if err := sysconfig.DisableCloudInit(); err != nil {
		return err
	}

	// 5. done, no output, no error indicates to initramfs we are done
	//    with mounting stuff
	return nil
}

func generateMountsModeRecover(recoverySystem string) error {
	return fmt.Errorf("recover mode mount generation not implemented yet")
}

func generateMountsModeRun() error {
	seedDir := filepath.Join(dirs.RunMnt, "ubuntu-seed")
	bootDir := filepath.Join(dirs.RunMnt, "ubuntu-boot")
	dataDir := filepath.Join(dirs.RunMnt, "ubuntu-data")

	// 1.1 always ensure basic partitions are mounted
	for _, d := range []string{seedDir, bootDir} {
		isMounted, err := osutilIsMounted(d)
		if err != nil {
			return err
		}
		if !isMounted {
			fmt.Fprintf(stdout, "/dev/disk/by-label/%s %s\n", filepath.Base(d), d)
		}
	}

	// 1.2 mount Data, and exit, as it needs to be mounted for us to do step 2
	isDataMounted, err := osutilIsMounted(dataDir)
	if err != nil {
		return err
	}
	if !isDataMounted {
		name := filepath.Base(dataDir)
		device, err := unlockIfEncrypted(name)
		if err != nil {
			return err
		}

		fmt.Fprintf(stdout, "%s %s\n", device, dataDir)
		return nil
	}

	// 2. check if base is mounted
	isBaseMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "base"))
	// 3. check if kernel is mounted
	isKernelMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "kernel"))
	// 4. check if snapd is mounted (only on first-boot will we mount it)
	isSnapdMounted, err := osutilIsMounted(filepath.Join(dirs.RunMnt, "snapd"))

	if !isBaseMounted || !isKernelMounted || !isSnapdMounted {
		// load the recovery system and generate mounts for kernel/base
		// and snapd
		var whichTypes []snap.Type
		if !isBaseMounted {
			whichTypes = append(whichTypes, snap.TypeBase)
		}
		if !isKernelMounted {
			whichTypes = append(whichTypes, snap.TypeKernel)
		}
		if !isSnapdMounted {
			whichTypes = append(whichTypes, snap.TypeSnapd)
		}

		mounts, err := boot.InitramfsRunModeSnapsToMount(whichTypes)
		if err != nil {
			return err
		}
		// TODO:UC20: should we have more cross-checks here?
		if _, ok := mounts[snap.TypeBase]; ok {
			fmt.Fprintf(stdout, "%s %s\n", mounts[snap.TypeBase], filepath.Join(dirs.RunMnt, "base"))
		}
		if _, ok := mounts[snap.TypeKernel]; ok {
			fmt.Fprintf(stdout, "%s %s\n", mounts[snap.TypeKernel], filepath.Join(dirs.RunMnt, "kernel"))
		}
		if _, ok := mounts[snap.TypeSnapd]; ok {
			fmt.Fprintf(stdout, "%s %s\n", mounts[snap.TypeSnapd], filepath.Join(dirs.RunMnt, "snapd"))
		}
	}

	return nil
}

func generateInitramfsMounts() error {
	mode, recoverySystem, err := boot.ModeAndRecoverySystemFromKernelCommandLine()
	if err != nil {
		return err
	}
	switch mode {
	case "recover":
		return generateMountsModeRecover(recoverySystem)
	case "install":
		return generateMountsModeInstall(recoverySystem)
	case "run":
		return generateMountsModeRun()
	}
	// this should never be reached
	return fmt.Errorf("internal error: mode in generateInitramfsMounts not handled")
}

func unlockIfEncrypted(name string) (string, error) {
	// TODO:UC20: will need to unseal key to unlock LUKS here
	device := filepath.Join("/dev/disk/by-label", name)
	keyfile := filepath.Join(dirs.RunMnt, "ubuntu-boot", name+".keyfile.unsealed")
	if osutil.FileExists(keyfile) {
		// TODO:UC20: snap-bootstrap should validate that <name>-enc is what
		//            we expect (and not e.g. an external disk), and also that
		//            <name> is from <name>-enc and not an unencrypted partition
		//            with the same name (LP #1863886)
		cmd := exec.Command("/usr/lib/systemd/systemd-cryptsetup", "attach", name, device+"-enc", keyfile)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "SYSTEMD_LOG_TARGET=console")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", osutil.OutputErr(output, err)
		}
	}
	return device, nil
}
