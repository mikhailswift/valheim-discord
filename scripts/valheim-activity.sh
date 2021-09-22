#!/bin/bash
set -e

WEBHOOK_URL="${1}"
RUN_DIR="/run/valheim-activity"
LAST_ACTIVITY_FILE="${RUN_DIR}/lastactivity"
STATUS_URL="http://localhost:8080/status.json"
TIMEOUT_MINS=60
TIMEOUT_SECS=$(( 60 * TIMEOUT_MINS ))

if [[ ! -d "${RUN_DIR}" ]]; then
	mkdir -p "${RUN_DIR}"
fi

if [[ ! -f "${LAST_ACTIVITY_FILE}" ]]; then
	date +%s > "${LAST_ACTIVITY_FILE}"
fi

NUM_PLAYERS=$(curl -s "${STATUS_URL}" | jq '.player_count // 0')
if (( NUM_PLAYERS > 0 )); then
	date +%s > "${LAST_ACTIVITY_FILE}"
	echo "${NUM_PLAYERS} currently active, doing nothing"
	exit 0
fi

LAST_ACTIVITY=$(cat "${LAST_ACTIVITY_FILE}")
NOW_SECS=$(date +%s)
DIFF_SECS=$(( NOW_SECS - LAST_ACTIVITY ))
DIFF_MINS=$(( DIFF_SECS / 60 ))
if (( DIFF_SECS > TIMEOUT_SECS )); then
	echo "${DIFF_MINS} minutes have elapsed with no activity, shutting down"
	/usr/local/bin/send-discord-message.sh "${WEBHOOK_URL}" "Shutting down server after ${DIFF_MINS} minutes of inactivity"
	systemctl poweroff
	exit 0
fi	

echo "$(( TIMEOUT_MINS - DIFF_MINS )) minutes left before shut down"
exit 0
