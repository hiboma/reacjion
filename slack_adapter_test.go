package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func TestSocketModeReacjiJSON(t *testing.T) {
	for _, tt := range []struct {
		name, bot, team, app, channel, icons string
		posts                                int
	}{
		{"trusted emoji", testReacjiB, testTeam, "A0APP", "C0CHAN", `{"emoji":"smile"}`, 1},
		{"spoofed bot", "B0EVIL", testTeam, "A0APP", "C0CHAN", `{"emoji":"smile"}`, 0},
		{"foreign team", testReacjiB, "T0EVIL", "A0APP", "C0CHAN", `{"emoji":"smile"}`, 0},
		{"foreign app", testReacjiB, testTeam, "A0EVIL", "C0CHAN", `{"emoji":"smile"}`, 0},
		{"other channel", testReacjiB, testTeam, "A0APP", "C0OTHER", `{"emoji":"smile"}`, 0},
		{"missing icons", testReacjiB, testTeam, "A0APP", "C0CHAN", `null`, 0},
		{"icon_emoji is not Reacji emoji", testReacjiB, testTeam, "A0APP", "C0CHAN", `{"icon_emoji":"smile"}`, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wire := fmt.Sprintf(`{"type":"events_api","envelope_id":"env-test","payload":{"type":"event_callback","team_id":%q,"api_app_id":%q,"event_id":"EvRaw","event":{"type":"message","subtype":"bot_message","channel":%q,"bot_id":%q,"username":"Reacji Channeler","text":"test message","icons":%s}}}`, tt.team, tt.app, tt.channel, tt.bot, tt.icons)
			var request socketmode.Request
			if err := json.Unmarshal([]byte(wire), &request); err != nil {
				t.Fatal(err)
			}
			event, err := slackevents.ParseEvent(request.Payload, slackevents.OptionNoVerifyToken())
			if err != nil {
				t.Fatal(err)
			}
			if err := restoreReacjiIcon(&event, request.Payload); err != nil {
				t.Fatal(err)
			}
			app, poster, _ := newTestApp(t)
			app.guard.appID = "A0APP"
			app.guard.channelIDs = map[string]struct{}{"C0CHAN": {}}
			app.handleEventsAPI(event)
			app.disp.close()
			if got := poster.count(); got != tt.posts {
				t.Fatalf("posts = %d, want %d", got, tt.posts)
			}
		})
	}
}

func TestRestoreReacjiIconRejectsMalformedData(t *testing.T) {
	for _, payload := range []string{`{`, `{"event":{"icons":{"emoji":42}}}`, `{"event":{"icons":[]}}`} {
		event := messageEvent("EvBad", testTeam, reacji(testReacjiB, "smile"))
		if err := restoreReacjiIcon(&event, json.RawMessage(payload)); err == nil {
			t.Errorf("malformed payload was accepted: %s", payload)
		}
	}
}
