# reacjion

A Slack bot (Socket Mode) that posts a configured message when
[Reacji Channeler](https://reacji-channeler.builtbyslack.com/) shares a
message with a matching emoji.

## Configuration

`config.yaml` maps an emoji to a message. The bot refuses to start when the
file is missing, invalid, or has unknown keys.

```yaml
- emoji: smile
  message: I got smile
```

Environment variables:

| Name | Required | Description |
|---|---|---|
| `SLACK_BOT_TOKEN` | yes | Bot User OAuth Token (starts with `xoxb`) |
| `SLACK_APP_TOKEN` | yes | App-Level Token for Socket Mode (starts with `xapp`) |
| `REACJI_BOT_IDS` | yes | Comma separated bot IDs of Reacji Channeler (`B...`) |
| `REACJI_CHANNEL_IDS` | no | Comma separated channel IDs to react in. Empty means all channels |
| `SLACK_APP_ID` | no | Your app ID (`A...`). Events for other apps are ignored |

### Finding the bot ID of Reacji Channeler

Messages are authenticated by **bot ID**, not by the display name or icon,
because any app or webhook that can post to the channel can set those.

If `REACJI_BOT_IDS` does not contain the right ID, the bot logs:

```
event: type=message event=... result=rejected reason=untrusted_bot bot_id=B0123456789
```

Check that the message really came from Reacji Channeler, then add the
`bot_id` to `REACJI_BOT_IDS`. A message whose subtype is not accepted is logged
with `reason=not_allowed_bot_message` and its `subtype`.

### Trust boundary

Anyone who can configure Reacji Channeler rules or add a reaction can make
the bot post. Only messages from the allowed bot IDs, in this workspace
(`team_id` from `auth.test`), are accepted. On an Enterprise Grid org-level
install, the team ID check may reject all events.

### Limits

Slack API calls run on 2 workers with a 10-second timeout. When the queue is
full or the rate limit is exceeded, the event is dropped and logged as
`dropped: ... reason=queue_full|rate_limited`. Mentions have a smaller rate
limit than Reacji Channeler posts, so a flood of mentions cannot block them.

## Logging

Logs contain only the event type, the result, and a hashed event ID.
Message text is not logged.

> **Warning:** `-debug` logs raw Slack requests and responses, including the
> **bot token** and message text. Do not use it in production, and do not
> share or ship its logs.

## Container

The image runs as uid 10001. A mounted `config.yaml` must be readable by
that user.
