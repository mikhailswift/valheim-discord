#!/bin/bash
set -e

WEBHOOK_URL="${1}"
MESSAGE="${2}"

sendMessage() {
	JSON_PAYLOAD=$(cat<<EOF
{
  "username": "valheimbot",
  "content": "${1}"
}
EOF
)

	curl -sfsL -X POST -H "Content-Type: application/json" -d "${JSON_PAYLOAD}" "${2}"
}

sendMessage "${MESSAGE}" "${WEBHOOK_URL}"
