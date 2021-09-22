#!/usr/bin/env bash
set -ex

CLIENT_ID="${1}"
CLIENT_SECRET="${2}"
GUILD_ID="${3}"
CHANNEL_ID="${4}"

API_ENDPOINT="https://discord.com/api/v8"

OAUTH_RESP=$(curl \
  -u "${CLIENT_ID}":"${CLIENT_SECRET}" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials&scope=applications.commands.update" \
  "${API_ENDPOINT}/oauth2/token")

BEARER_TOKEN=$(jq -r '.access_token' <<< ${OAUTH_RESP})

COMMANDS=$(cat <<EOF
{
  "name": "valheim",
  "description": "Control the Fun with Friends Valheim server",
  "options": [
    {
      "type": 1,
      "name": "status",
      "description": "Gets the current status of the server"
    },
    {
      "type": 1,
      "name": "start",
      "description": "Starts the valheim server"
    },
    {
      "type": 1,
      "name": "stop",
      "description": "Stops the valheim server"
    }
  ]
}
EOF
)

CREATE_COMMANDS_RESP=$(curl \
  -H "Authorization: Bearer ${BEARER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${COMMANDS}" \
  "${API_ENDPOINT}/applications/${CLIENT_ID}/guilds/${GUILD_ID}/commands")
