# slack-bridge

Minimal Slack Socket Mode bridge for sending Slack requests to an OpenCode server and posting responses in Slack threads. An authorized user starts a conversation by mentioning the bot; authorized replies in that thread do not need another mention.

The image is a static Go binary running from `scratch`.

Required environment: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, and `SLACK_ALLOWED_USER_IDS`.

The Slack bot token needs the `reactions:write` scope so the bridge can mark a mention as processing with `:eyes:`, then replace it with `:white_check_mark:` after its response posts successfully.

Configure Slack event subscriptions for `app_mention` and the matching message event so the bridge receives unmentioned thread replies: `message.channels` for public channels or `message.groups` for private channels. The app needs the corresponding channel-history permission.

`SLACK_ALLOWED_USER_IDS` is a comma-separated list of Slack user IDs. Messages from users not in this list are ignored. An empty list denies all users.

Optional environment: `SLACK_CHANNEL_ID`, `OPENCODE_URL` (default `http://opencode:4096`), `OPENCODE_REQUEST_TIMEOUT` (default `30m`), `OPENCODE_SERVER_USERNAME`, and `OPENCODE_SERVER_PASSWORD`.

The bridge exposes `GET /healthz` on port `8080` for Kubernetes probes.

```bash
make build IMAGE=slack-bridge
```
