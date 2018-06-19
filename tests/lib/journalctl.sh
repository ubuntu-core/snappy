#!/bin/bash

JOURNALCTL_CURSOR_FILE="${SPREAD_PATH}"/journalctl_cursor

get_last_journalctl_cursor(){
    journalctl --output=export -n1 | grep --binary-files=text -o '__CURSOR=.*' | sed -e 's/^__CURSOR=//'
}

start_new_journalctl_log(){
    cursor=$(get_last_journalctl_cursor)
    if [ -z "$cursor" ]; then
        echo "Empty journalctl cursor, exiting..."
        exit 1
    else
        echo "$cursor" > "$JOURNALCTL_CURSOR_FILE"
    fi

    echo "New test starts here - $SPREAD_JOB" | systemd-cat
    test_id="test-${RANDOM}${RANDOM}"
    echo "$test_id" | systemd-cat
    if get_journalctl_log | grep -q "$test_id"; then
        return
    fi
    get_journalctl_log
    echo "Test id not found in journalctl, exiting..."
    exit 1
}

get_journalctl_log(){
    cursor=$(cat "$JOURNALCTL_CURSOR_FILE")
    get_journalctl_log_from_cursor "$cursor" "$@"
}

get_journalctl_log_from_cursor(){
    cursor=$1
    shift
    journalctl "$@" --cursor "$cursor"
}
