# slack-bridge

Minimal Slack Socket Mode bridge for sending `app_mention` events to an OpenCode server and posting responses in Slack threads.

The image is a static Go binary running from `scratch`.

Required environment: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`.

Optional environment: `SLACK_CHANNEL_ID`, `OPENCODE_URL` (default `http://opencode:4096`), `OPENCODE_SERVER_USERNAME`, and `OPENCODE_SERVER_PASSWORD`.

The bridge exposes `GET /healthz` on port `8080` for Kubernetes probes.

```bash
make build IMAGE=slack-bridge
```
