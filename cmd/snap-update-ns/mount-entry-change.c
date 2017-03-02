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

#include "mount-entry-change.h"

#include <stdlib.h>

#include "../libsnap-confine-private/utils.h"

void
sc_compute_required_mount_changes(struct sc_mount_entry * *desiredp,
				  struct sc_mount_entry * *currentp,
				  struct sc_mount_change *change)
{
	struct sc_mount_entry *d, *c;
	if (desiredp == NULL) {
		die("cannot compute required mount changes, desiredp is NULL");
	}
	if (currentp == NULL) {
		die("cannot compute required mount changes, currentp is NULL");
	}
	if (change == NULL) {
		die("cannot compute required mount changes, change is NULL");
	}
	d = *desiredp;
	c = *currentp;
	bool again;
	do {
		again = false;
		if (c == NULL && d == NULL) {
			// Both profiles exhausted. There is nothing to do left.
			change->action = SC_ACTION_NONE;
			change->entry = NULL;
			*currentp = NULL;
			*desiredp = NULL;
		} else if (c == NULL && d != NULL) {
			// Current profile exhausted but desired profile is not.
			// Generate a MOUNT action based on desired profile and advance it.
			change->action = SC_ACTION_MOUNT;
			change->entry = d;
			*currentp = NULL;
			*desiredp = d->next;
		} else if (c != NULL && d == NULL) {
			// Current profile is not exhausted but the desired profile is.
			// Generate an UNMOUNT action based on the current profile and advance it.
			change->action = SC_ACTION_UNMOUNT;
			change->entry = c;
			*currentp = c->next;
			*desiredp = NULL;
		} else if (c != NULL && d != NULL) {
			// Both profiles have entries to consider.
			if (sc_compare_mount_entry(c, d) == 0) {
				// Identical entries are just skipped and the algorithm continues.
				c = c->next;
				d = d->next;
				// Do another pass over the algorithm.
				again = true;
				continue;
			} else {
				// Non-identail entries mean that we need to unmount the current
				// entry and mount the desired entry.
				//
				// Let's process all the unmounts first. This way we can "clear the
				// stage" (so to speak). Either the tip of the current profile and
				// tip of the desired profile become identical (we're in sync) or
				// we're eventually going to exhaust the current profile and the
				// code above will start to process items in the desired profile
				// (which will cause all the mount calls to happen).
				change->action = SC_ACTION_UNMOUNT;
				change->entry = c;
				*currentp = c->next;
				*desiredp = d;
			}
		}
	} while (again);
}
