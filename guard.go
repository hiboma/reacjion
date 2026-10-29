package main

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/slack-go/slack/slackevents"
)

const reacjiUsername = "Reacji Channeler"

// guard decides whether an event comes from a trusted source.
//
// The display name and the icon of a message can be set by any app or
// incoming webhook that can post to the channel, so they are never used
// to authenticate the sender. The bot ID is.
type guard struct {
	teamID     string              // our workspace, from auth.test
	appID      string              // our app; optional
	botIDs     map[string]struct{} // bot IDs of Reacji Channeler
	channelIDs map[string]struct{} // empty means all channels
}

// checkOuter verifies the envelope of an Events API event.
func (g *guard) checkOuter(ev slackevents.EventsAPIEvent) bool {
	if g.teamID != "" && ev.TeamID != g.teamID {
		return false
	}
	if g.appID != "" && ev.APIAppID != g.appID {
		return false
	}
	return true
}

func (g *guard) channelAllowed(channel string) bool {
	if len(g.channelIDs) == 0 {
		return true
	}
	_, ok := g.channelIDs[channel]
	return ok
}

type verdict int

const (
	verdictIgnore  verdict = iota // not a Reacji Channeler message
	verdictSpoofed                // looks like Reacji Channeler, but the bot ID is not allowed
	verdictChannel                // allowed bot, but the channel is not allowed
	verdictTrusted
)

// checkReacji reports whether the message was posted by an allowed
// Reacji Channeler bot.
func (g *guard) checkReacji(e *slackevents.MessageEvent) verdict {
	if e == nil || e.BotID == "" {
		return verdictIgnore
	}
	// Reacji Channeler posts as "bot_message"; a plain bot post has no subtype.
	// Edits, deletions and other subtypes are never handled.
	if e.SubType != "" && e.SubType != "bot_message" {
		return verdictIgnore
	}
	if _, ok := g.botIDs[e.BotID]; !ok {
		if e.Username == reacjiUsername {
			return verdictSpoofed
		}
		return verdictIgnore
	}
	if !g.channelAllowed(e.Channel) {
		return verdictChannel
	}
	return verdictTrusted
}

// iconEmoji returns the icon emoji of a message, or "" if it has none.
func iconEmoji(e *slackevents.MessageEvent) string {
	if e == nil || e.Icons == nil {
		return ""
	}
	return e.Icons.Emoji
}

func matchCallbacks(callbacks []Callback, emoji string) []Callback {
	if emoji == "" {
		return nil
	}
	var matched []Callback
	for _, c := range callbacks {
		if c.Emoji == emoji {
			matched = append(matched, c)
		}
	}
	return matched
}

// hashID makes an ID usable in logs without exposing it as is.
func hashID(id string) string {
	if id == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:6])
}
