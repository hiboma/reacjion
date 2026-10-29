package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type fakePoster struct {
	mu       sync.Mutex
	channels []string
	err      error
}

func (f *fakePoster) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels = append(f.channels, channelID)
	return "", "", f.err
}

func (f *fakePoster) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.channels)
}

const (
	testTeam    = "T0TEAM"
	testReacjiB = "B0REACJI"
	secretText  = "very secret message body"
)

func newTestApp(t *testing.T) (*App, *fakePoster, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	p := &fakePoster{}
	app := &App{
		callbacks: []Callback{{Emoji: "smile", Message: "I got smile"}},
		api:       p,
		logger:    log.New(buf, "", 0),
		dedup:     newDedup(time.Minute, 100),
		guard: &guard{
			teamID: testTeam,
			botIDs: map[string]struct{}{testReacjiB: {}},
		},
	}
	app.disp = newDispatcher(1, 10, time.Second, app.logger.Printf)
	app.reacjiRL = newRateLimiter(1, 10)
	app.mentionRL = newRateLimiter(0.2, 3)
	return app, p, buf
}

func messageEvent(eventID, team string, m *slackevents.MessageEvent) slackevents.EventsAPIEvent {
	return slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: team,
		Data:   &slackevents.EventsAPICallbackEvent{EventID: eventID, TeamID: team},
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: m,
		},
	}
}

func reacji(botID, emoji string) *slackevents.MessageEvent {
	m := &slackevents.MessageEvent{
		Channel:  "C0CHAN",
		Text:     secretText,
		SubType:  "bot_message",
		BotID:    botID,
		Username: reacjiUsername,
	}
	if emoji != "" {
		m.Icons = &slackevents.Icon{Emoji: emoji}
	}
	return m
}

func TestHandleEventsAPI(t *testing.T) {
	tests := []struct {
		name  string
		event slackevents.EventsAPIEvent
		posts int
	}{
		{"trusted reacji posts", messageEvent("Ev1", testTeam, reacji(testReacjiB, "smile")), 1},
		{"spoofed username is rejected", messageEvent("Ev2", testTeam, reacji("B0EVIL", "smile")), 0},
		{"other team is rejected", messageEvent("Ev3", "T0OTHER", reacji(testReacjiB, "smile")), 0},
		{"nil icons does not panic", messageEvent("Ev4", testTeam, reacji(testReacjiB, "")), 0},
		{"unknown emoji is ignored", messageEvent("Ev5", testTeam, reacji(testReacjiB, "cry")), 0},
		{"user message is ignored", messageEvent("Ev6", testTeam, &slackevents.MessageEvent{
			Channel: "C0CHAN", Text: secretText, User: "U0USER", Username: reacjiUsername,
			Icons: &slackevents.Icon{Emoji: "smile"},
		}), 0},
		{"nil message does not panic", messageEvent("Ev7", testTeam, nil), 0},
		{"plain bot post without subtype posts", messageEvent("Ev8", testTeam, func() *slackevents.MessageEvent {
			m := reacji(testReacjiB, "smile")
			m.SubType = ""
			return m
		}()), 1},
		{"message_changed is ignored", messageEvent("Ev9", testTeam, func() *slackevents.MessageEvent {
			m := reacji(testReacjiB, "smile")
			m.SubType = "message_changed"
			return m
		}()), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, p, buf := newTestApp(t)
			app.handleEventsAPI(tt.event)
			app.disp.close()
			if got := p.count(); got != tt.posts {
				t.Errorf("posts = %d, want %d", got, tt.posts)
			}
			if strings.Contains(buf.String(), secretText) {
				t.Errorf("message text leaked to logs: %s", buf.String())
			}
		})
	}
}

func TestHandleEventsAPIDeduplicates(t *testing.T) {
	app, p, _ := newTestApp(t)
	ev := messageEvent("EvSame", testTeam, reacji(testReacjiB, "smile"))
	app.handleEventsAPI(ev)
	app.handleEventsAPI(ev)
	app.disp.close()
	if got := p.count(); got != 1 {
		t.Errorf("posts = %d, want 1", got)
	}
}

func TestGuardChannelRestriction(t *testing.T) {
	g := &guard{
		botIDs:     map[string]struct{}{testReacjiB: {}},
		channelIDs: map[string]struct{}{"C0ALLOWED": {}},
	}
	m := reacji(testReacjiB, "smile")
	if got := g.checkReacji(m); got != verdictChannel {
		t.Errorf("verdict = %v, want channel for a channel not in the list", got)
	}
	m.Channel = "C0ALLOWED"
	if got := g.checkReacji(m); got != verdictTrusted {
		t.Errorf("verdict = %v, want trusted", got)
	}
}

func TestGuardCheckOuterAppID(t *testing.T) {
	g := &guard{teamID: testTeam, appID: "A0APP"}
	if g.checkOuter(slackevents.EventsAPIEvent{TeamID: testTeam, APIAppID: "A0OTHER"}) {
		t.Error("event for another app was accepted")
	}
	if !g.checkOuter(slackevents.EventsAPIEvent{TeamID: testTeam, APIAppID: "A0APP"}) {
		t.Error("event for our app was rejected")
	}
}

func TestParseCallbacks(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{"valid", "- emoji: smile\n  message: hi\n", false},
		{"broken yaml", "- emoji: [smile\n", true},
		{"unknown key", "- emoji: smile\n  mesage: typo\n", true},
		{"empty", "", true},
		{"empty emoji", "- emoji: ''\n  message: hi\n", true},
		{"empty message", "- emoji: smile\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCallbacks([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestParseIDList(t *testing.T) {
	ids := parseIDList(" B01, ,B02 ")
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2", len(ids))
	}
	if _, ok := ids["B02"]; !ok {
		t.Error("B02 is missing")
	}
}

func TestDispatcherDropsWhenFull(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 2)
	d := newDispatcher(1, 1, time.Second, func(string, ...interface{}) {})
	slow := job{kind: "test", run: func(ctx context.Context) error { started <- struct{}{}; <-block; return nil }}

	if !d.submit(slow, nil) { // taken by the worker
		t.Fatal("first job was dropped")
	}
	<-started
	if !d.submit(slow, nil) { // fills the queue
		t.Fatal("second job was dropped")
	}
	if d.submit(slow, nil) { // queue is full
		t.Error("third job was accepted, want dropped")
	}
	close(block)
	d.close()
}

func TestDispatcherTimesOut(t *testing.T) {
	var gotErr error
	var mu sync.Mutex
	d := newDispatcher(1, 1, 20*time.Millisecond, func(string, ...interface{}) {})
	d.submit(job{kind: "test", run: func(ctx context.Context) error {
		<-ctx.Done()
		mu.Lock()
		gotErr = ctx.Err()
		mu.Unlock()
		return ctx.Err()
	}}, nil)
	d.close()
	if gotErr != context.DeadlineExceeded {
		t.Errorf("err = %v, want deadline exceeded", gotErr)
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	r := newRateLimiter(1, 2)
	r.now = func() time.Time { return now }
	r.last = now

	if !r.allow() || !r.allow() {
		t.Fatal("burst was not allowed")
	}
	if r.allow() {
		t.Error("allowed beyond burst")
	}
	now = now.Add(time.Second)
	if !r.allow() {
		t.Error("token was not refilled")
	}
}

func TestDedup(t *testing.T) {
	now := time.Unix(0, 0)
	d := newDedup(time.Minute, 2)
	d.now = func() time.Time { return now }

	if !d.firstTime("a") || d.firstTime("a") {
		t.Fatal("duplicate was not detected")
	}
	now = now.Add(2 * time.Minute)
	if !d.firstTime("a") {
		t.Error("expired entry was treated as duplicate")
	}
	d.firstTime("b")
	d.firstTime("c") // exceeds max; must not grow without bound
	if len(d.seen) > 2 {
		t.Errorf("len = %d, want <= 2", len(d.seen))
	}
}

func TestMentionFloodDoesNotStarveReacji(t *testing.T) {
	app, p, _ := newTestApp(t)
	for i := 0; i < 20; i++ {
		app.handleEventsAPI(slackevents.EventsAPIEvent{
			Type:   slackevents.CallbackEvent,
			TeamID: testTeam,
			Data:   &slackevents.EventsAPICallbackEvent{EventID: fmt.Sprintf("EvM%d", i)},
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "app_mention",
				Data: &slackevents.AppMentionEvent{Channel: "C0MENTION", Text: "hi"},
			},
		})
	}
	app.handleEventsAPI(messageEvent("EvR", testTeam, reacji(testReacjiB, "smile")))
	app.disp.close()

	p.mu.Lock()
	defer p.mu.Unlock()
	mentions, reacjis := 0, 0
	for _, c := range p.channels {
		if c == "C0MENTION" {
			mentions++
		} else {
			reacjis++
		}
	}
	if mentions != 3 {
		t.Errorf("mentions = %d, want 3 (burst)", mentions)
	}
	if reacjis != 1 {
		t.Errorf("reacji posts = %d, want 1", reacjis)
	}
}

func TestRejectedSpoofIsLoggedWithoutText(t *testing.T) {
	app, _, buf := newTestApp(t)
	app.handleEventsAPI(messageEvent("EvS", testTeam, reacji("B0EVIL", "smile")))
	app.disp.close()
	if !strings.Contains(buf.String(), `reason=untrusted_bot bot_id="B0EVIL"`) {
		t.Errorf("rejection was not logged: %s", buf.String())
	}
}

func TestMentionAndAPIErrorDoNotLogText(t *testing.T) {
	app, p, buf := newTestApp(t)
	p.err = errors.New("channel_not_found")
	app.handleEventsAPI(slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: testTeam,
		Data:   &slackevents.EventsAPICallbackEvent{EventID: "EvM"},
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{Channel: "C0CHAN", Text: secretText},
		},
	})
	app.handleEventsAPI(messageEvent("EvR", testTeam, reacji(testReacjiB, "smile")))
	app.disp.close()
	logs := buf.String()
	if !strings.Contains(logs, "failed: kind=reacji") || !strings.Contains(logs, "channel_not_found") {
		t.Errorf("API error was not logged: %s", logs)
	}
	if strings.Contains(logs, secretText) {
		t.Errorf("message text leaked to logs: %s", logs)
	}
}

func TestDroppedEventCanBeRedelivered(t *testing.T) {
	app, p, _ := newTestApp(t)
	app.reacjiRL = newRateLimiter(0, 0) // drop everything
	ev := messageEvent("EvDrop", testTeam, reacji(testReacjiB, "smile"))
	app.handleEventsAPI(ev)
	app.reacjiRL = newRateLimiter(1, 1)
	app.handleEventsAPI(ev) // redelivery
	app.disp.close()
	if got := p.count(); got != 1 {
		t.Errorf("posts = %d, want 1 after redelivery", got)
	}
}

func TestRateLimiterRefund(t *testing.T) {
	now := time.Unix(0, 0)
	r := newRateLimiter(0, 1)
	r.now = func() time.Time { return now }
	r.last = now
	if !r.allow() {
		t.Fatal("first token was not allowed")
	}
	r.refund()
	if !r.allow() {
		t.Error("refunded token was not allowed")
	}
	r.refund()
	r.refund()
	if r.tokens > r.burst {
		t.Errorf("tokens = %v, want <= burst %v", r.tokens, r.burst)
	}
}
