/*
 * Copyright (C) 2018 Canonical Ltd
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

#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

#include "../libsnap-confine-private/utils.h"

int must_parse_int(const char *s)
{
	char *endptr = NULL;

	errno = 0;
	long i = strtol(s, &endptr, 10);
	if (errno != 0 || s == endptr || *endptr != '\0' || i < 0) {
		die("cannot parse number in '%s'", s);
	}
	return i;
}

int main(int argc, char **argv)
{
	if (sc_is_debug_enabled()) {
		for (int i = 0; i < argc; i++) {
			printf("-%s-\n", argv[i]);
		}
	}

	if (getuid() == 0) {
		// check if we run as SUDO and if so switch to a normal user
		const char *sudo_uid_env = getenv("SUDO_UID");
		if (sudo_uid_env != NULL) {
			int sudo_uid = must_parse_int(sudo_uid_env);
			if (sudo_uid != 0) {
				if (setuid(sudo_uid) != 0) {
					die("cannot switch to uid %d",
					    sudo_uid);
				}
			}
		}
	}
	if (getgid() == 0) {
		// check if we run as SUDO and if so switch to a normal user
		const char *sudo_gid_env = getenv("SUDO_GID");
		if (sudo_gid_env != NULL) {
			int sudo_gid = must_parse_int(sudo_gid_env);
			if (sudo_gid != 0) {
				if (setgid(sudo_gid) != 0) {
					die("cannot switch to gid %d",
					    sudo_gid);
				}
			}
		}
	}
	// Ideally we would also call setgroups() now but seccomp will
	// prevent this. At this point we are inside the confinement
	// of the snap already.

	// signal gdb to stop here
	printf("\n\n");
	printf("Welcome to `snap run --gdb`.\n");
	printf("You are right before your application is execed():\n");
	printf("- set any options you may need\n");
	printf("- use 'cont' to start\n");
	printf("\n\n");
	raise(SIGTRAP);

	const char *executable = argv[1];
	execv(executable, (char *const *)&argv[1]);
	perror("execv failed");
	// very different exit code to make an execve failure easy to distinguish
	return 101;
}
