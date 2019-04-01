#include "config.h"
#include "classic.h"
#include "../libsnap-confine-private/cleanup-funcs.h"
#include "../libsnap-confine-private/string-utils.h"
#include "../libsnap-confine-private/utils.h"

#include <stdarg.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

static const char *os_release = "/etc/os-release";
static const char *meta_snap_yaml = "/meta/snap.yaml";

sc_distro sc_classify_distro(void)
{
	char *id SC_CLEANUP(sc_cleanup_string) = NULL;
	char *version_id SC_CLEANUP(sc_cleanup_string) = NULL;
	char *variant_id SC_CLEANUP(sc_cleanup_string) = NULL;

	sc_probe_distro(os_release, "ID", &id, "VERSION_ID", &version_id,
			"VARIANT_ID", &variant_id, NULL);

	bool is_core = false;
	int core_version = 0;

	if (sc_streq(id, "\"ubuntu-core\"") || sc_streq(id, "ubuntu-core")) {
		is_core = true;
	}
	if (sc_streq(variant_id, "\"snappy\"") || sc_streq(variant_id, "snappy")) {
		is_core = true;
	}
	if (sc_streq(version_id, "\"16\"") || sc_streq(version_id, "16")) {
		core_version = 16;
	}
	if (!is_core) {
		/* Since classic systems don't have a /meta/snap.yaml file the simple
		   presence of that file qualifies as SC_DISTRO_CORE_OTHER. */
		if (access(meta_snap_yaml, F_OK) == 0) {
			is_core = true;
		}
	}

	if (is_core) {
		if (core_version == 16) {
			return SC_DISTRO_CORE16;
		}
		return SC_DISTRO_CORE_OTHER;
	} else {
		return SC_DISTRO_CLASSIC;
	}
}

void sc_probe_distro(const char *os_release_path, ...)
{
	FILE *f SC_CLEANUP(sc_cleanup_file) = fopen(os_release_path, "r");
	if (f == NULL) {
		die("cannot open %s", os_release);
	}

	va_list ap;
	va_start(ap, os_release_path);
	for (;;) {
		const char *key = va_arg(ap, const char *);
		if (key == NULL) {
			break;
		}
		char **value = va_arg(ap, char **);
		if (value != NULL) {
			*value = NULL;
		}

		fseek(f, 0, SEEK_SET);
		char buf[255] = { 0 };
		while (fgets(buf, sizeof buf, f) != NULL) {
			size_t len = strlen(buf);
			if (len > 0 && buf[len - 1] == '\n') {
				buf[len - 1] = '\0';
			}
			if (strstr(buf, key) == buf && buf[strlen(key)] == '=') {
				if (value != NULL) {
					*value =
					    sc_strdup(buf + strlen(key) + 1);
				}
				break;
			}
		}
	}
	va_end(ap);
}

bool sc_should_use_normal_mode(sc_distro distro, const char *base_snap_name)
{
	return distro != SC_DISTRO_CORE16 || !sc_streq(base_snap_name, "core");
}
