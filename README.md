# valheimbot

This is a bot my friends and I use to control our Valheim server hosted on GCP.
It consists of a Cloud Function that serves as the backend of a Discord
Application.  The script `scripts/create_commands.sh` will manually create the
commands for the specified guild because I couldn't be bothered to implement the
entire OAuth2 workflow. The commands are `/valheim status`, `/valheim start`,
and `/valheim stop` which should be pretty self explanatory.

We wanted to confine the bot's communications to a specific channel in our guild
so all responses to the commands are sent as Ephemeral messages that only the
invoking user can see.  Status updates will be sent to a provided webhook that
is limited to the channel where updates should be sent.

## Configuration
The Cloud Function relies on a handful of environment variables to be set.

| Variable | Description |
|----------|-------------|
| `DISCORD_PUBKEY`      | This is the hex encoded public key discord provides your application |
| `DISCORD_WEBHOOK_URL` | The URL of the webhook the bot will use to send updates |
| `GCP_PROJECT`         | The project your valheim compute instances reside in |
| `GCP_ZONE`            | The zone your valheim compute instance resides in |
| `GCP_INSTANCE_NAME`   | The name of your compute instance that is running the valheim server |
| `STATUS_SERVER_PORT`  | The port of the valheim status server that is running on the compute instance |

## Utilities & scripts
The `scripts` directory contains a few helpful utilities.

As mentioned above the `create_commands.sh` script will bootstrap the commands
in your guild.

`send-discord-message` will publish a message as valheimbot to a provided
webhook.  This can be used on the GCP instance to provide updates if needed.

`valheim-activity.sh` will monitor the valheim server's player count and shut
the GCP instance off after a set amount of time.  Also provided are a systemd
unit and timer to run this script every 10 minutes.  It is dependent on the
Valheim status server from [lloesche's docker
image](https://github.com/lloesche/valheim-server-docker) running, though it could be
modified like [this](https://github.com/lloesche/valheim-server-docker/blob/777f2be033f9c35fedfca45f19635b35b2869d1e/common#L139)
to work in cases where the status server isn't running.

## Other info
Feel free to use and adapt these scripts for your own usage, but be warned this
code carries no support or guarantees from me.  I hacked it together without
re-use in mind but did try to make sane choices that didn't leave hard coded
values lying around.

I run my Valheim server with [lloesche's Docker
image](https://github.com/lloesche/valheim-server-docker/).  I cannot make
guarantees that this would work with other server configurations but I don't see
why it wouldn't.
