# slack-bridge

Minimal Slack Socket Mode bridge for sending Slack requests to an OpenCode server and posting responses in Slack threads or direct messages. An authorized user starts a channel conversation by mentioning the bot; authorized replies in that thread do not need another mention. Permission and question requests are posted back to the originating conversation with interactive controls, so a request waiting for input is visible rather than appearing stalled.

The image is a static Go binary running from `scratch`.

Required environment: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, and `SLACK_ALLOWED_USER_IDS`.

The Slack bot token needs the `reactions:write` scope so the bridge can mark a mention as processing with `:eyes:`, then replace it with `:white_check_mark:` after its response posts successfully. It also needs `chat:write` and interactivity must be enabled for the Slack app; Socket Mode delivers the button actions back to the bridge.

Configure Slack event subscriptions for `app_mention` and the matching message event so the bridge receives unmentioned thread replies: `message.channels` for public channels or `message.groups` for private channels. The app needs the corresponding channel-history permission.

To enable direct messages, turn on the app's App Home Messages Tab, subscribe to `message.im`, grant the bot `im:history`, and set `SLACK_ALLOW_DMS=true`. Direct messages are handled as one continuous OpenCode session per Slack DM and responses are posted directly in the conversation. `SLACK_ALLOWED_USER_IDS` applies to direct messages too.

`SLACK_ALLOWED_USER_IDS` is a comma-separated list of Slack user IDs. Messages from users not in this list are ignored. An empty list denies all users.

Optional environment: `SLACK_CHANNEL_ID`, `SLACK_ALLOW_DMS` (default `false`), `OPENCODE_URL` (default `http://opencode:4096`), `OPENCODE_REQUEST_TIMEOUT` (default `30m`), `OPENCODE_SERVER_USERNAME`, and `OPENCODE_SERVER_PASSWORD`. `SLACK_CHANNEL_ID` accepts a comma-separated list of channel IDs; leave it unset to allow all channels.

The bridge exposes `GET /healthz` on port `8080` for Kubernetes probes.

```bash
make build IMAGE=slack-bridge
```
