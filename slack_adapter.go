package main

import (
	"encoding/json"

	"github.com/slack-go/slack/slackevents"
)

// restoreReacjiIcon preserves Reacji Channeler's icons.emoji field, which the
// upstream slack-go SDK does not decode (it expects icons.icon_emoji instead).
// Only icon data is adapted; sender and envelope authentication use the SDK's
// original fields. The caller passes the same Socket Mode request's raw payload.
func restoreReacjiIcon(ev *slackevents.EventsAPIEvent, payload json.RawMessage) error {
	message, ok := ev.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok || message == nil {
		return nil
	}
	var raw struct {
		Event struct {
			Icons *struct {
				Emoji string `json:"emoji"`
			} `json:"icons"`
		} `json:"event"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}
	// Copy rather than mutate the SDK event that may have other consumers.
	adapted := *message
	adapted.Icons = nil
	if raw.Event.Icons != nil {
		adapted.Icons = &slackevents.Icon{IconEmoji: raw.Event.Icons.Emoji}
	}
	ev.InnerEvent.Data = &adapted
	return nil
}
