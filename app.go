package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	apiTimeout = 10 * time.Second
	workers    = 2
	queueSize  = 32
	// Reacji Channeler posts are driven by reactions; mentions can be sent
	// by any member, so they get a smaller budget.
	reacjiRatePerSec  = 1.0
	reacjiBurst       = 10
	mentionRatePerSec = 0.2
	mentionBurst      = 3
	dedupTTL          = 10 * time.Minute
	dedupMaxSize      = 10000
)

// poster is the part of the Slack API that the handlers use.
type poster interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

type App struct {
	callbacks []Callback
	client    *socketmode.Client
	api       poster
	guard     *guard
	disp      *dispatcher
	dedup     *dedup
	reacjiRL  *rateLimiter
	mentionRL *rateLimiter
	Debug     bool
	logger    *log.Logger
}

func (app *App) Initalize() {
	app.logger = log.New(os.Stdout, "[reacjion  ] ", log.LstdFlags)
	app.dedup = newDedup(dedupTTL, dedupMaxSize)
	app.reacjiRL = newRateLimiter(reacjiRatePerSec, reacjiBurst)
	app.mentionRL = newRateLimiter(mentionRatePerSec, mentionBurst)
}

func (app *App) InitalizeCallbacks() {
	callbacks, err := loadCallbacks("config.yaml")
	if err != nil {
		app.logger.Fatalf("invalid config.yaml: %s", err)
	}
	app.callbacks = callbacks
	app.logger.Printf("initialized %d callbacks", len(app.callbacks))
}

func (app *App) InitializeSlack() {
	if app.Debug {
		app.logger.Printf("WARNING: -debug logs raw Slack requests and payloads, including the bot token and message text. Do not use it in production or share its logs.")
	}

	botIDs := parseIDList(os.Getenv("REACJI_BOT_IDS"))
	if len(botIDs) == 0 {
		app.logger.Fatalf("REACJI_BOT_IDS is not set: set the bot ID(s) of Reacji Channeler (e.g. B0123456789)")
	}

	restClient := slack.New(
		os.Getenv("SLACK_BOT_TOKEN"),
		slack.OptionAppLevelToken(os.Getenv("SLACK_APP_TOKEN")),
		slack.OptionHTTPClient(&http.Client{Timeout: apiTimeout}),
		slack.OptionDebug(app.Debug),
		slack.OptionLog(log.New(os.Stderr, "[RESTclient] ", log.LstdFlags)),
	)

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()
	auth, err := restClient.AuthTestContext(ctx)
	if err != nil {
		app.logger.Fatalf("auth.test failed: %s", err)
	}

	app.guard = &guard{
		teamID:     auth.TeamID,
		appID:      strings.TrimSpace(os.Getenv("SLACK_APP_ID")),
		botIDs:     botIDs,
		channelIDs: parseIDList(os.Getenv("REACJI_CHANNEL_IDS")),
	}
	app.logger.Printf("allowed bots=%d channels=%d app_id_check=%t",
		len(app.guard.botIDs), len(app.guard.channelIDs), app.guard.appID != "")

	app.client = socketmode.New(
		restClient,
		socketmode.OptionDebug(app.Debug),
		socketmode.OptionLog(log.New(os.Stderr, "[socketmode] ", log.LstdFlags)),
	)
	app.api = app.client
}

func (app *App) RunLoop() {
	app.disp = newDispatcher(workers, queueSize, apiTimeout, app.logger.Printf)

	go func() {
		// This loop only acks and enqueues, so it never waits for the Slack Web API.
		for event := range app.client.Events {
			if event.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			if event.Request != nil {
				app.client.Ack(*event.Request)
			}
			eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
			if !ok {
				app.logger.Printf("event: result=ignored reason=unexpected_data")
				continue
			}
			app.handleEventsAPI(eventsAPIEvent)
		}
	}()

	if err := app.client.Run(); err != nil {
		app.logger.Fatalf("socketmode: %s", err)
	}
}

// handleEventsAPI checks an event and enqueues the work for it.
// It logs only the event type, the result and a hashed event ID,
// never the message itself.
func (app *App) handleEventsAPI(ev slackevents.EventsAPIEvent) {
	if ev.Type != slackevents.CallbackEvent {
		return
	}

	var rawID string
	if cb, ok := ev.Data.(*slackevents.EventsAPICallbackEvent); ok {
		rawID = cb.EventID
	}
	id := hashID(rawID)

	if !app.guard.checkOuter(ev) {
		app.logger.Printf("event: type=%s event=%s result=ignored reason=foreign_team_or_app", ev.InnerEvent.Type, id)
		return
	}

	switch e := ev.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		app.handleMention(rawID, e)
	case *slackevents.MessageEvent:
		app.handleMessage(rawID, e)
	}
}

// firstTime records an event that will be acted on and reports whether it
// is new. Only such events are recorded, so unrelated channel traffic does
// not push them out of the dedup table.
func (app *App) firstTime(kind, rawID string) bool {
	if app.dedup.firstTime(rawID) {
		return true
	}
	app.logger.Printf("event: type=%s event=%s result=ignored reason=duplicate", kind, hashID(rawID))
	return false
}

func (app *App) handleMention(rawID string, e *slackevents.AppMentionEvent) {
	id := hashID(rawID)
	if e == nil || !app.guard.channelAllowed(e.Channel) {
		app.logger.Printf("event: type=app_mention event=%s result=ignored reason=channel", id)
		return
	}
	if !app.firstTime("app_mention", rawID) {
		return
	}

	message := "pong"
	if strings.Contains(e.Text, "help") {
		message = "sorry, help is not prepared"
	}

	channel := e.Channel
	app.disp.submit(job{eventID: id, kind: "app_mention", run: func(ctx context.Context) error {
		_, _, err := app.api.PostMessageContext(ctx, channel, slack.MsgOptionText(message, true))
		return err
	}}, app.mentionRL)
}

func (app *App) handleMessage(rawID string, e *slackevents.MessageEvent) {
	id := hashID(rawID)
	switch app.guard.checkReacji(e) {
	case verdictIgnore:
		if e != nil && e.Username == reacjiUsername {
			// Helps to diagnose a legitimate message that is not accepted.
			app.logger.Printf("event: type=message event=%s result=ignored reason=not_allowed_bot_message subtype=%q bot_id=%s", id, e.SubType, e.BotID)
		}
		return
	case verdictSpoofed:
		// The bot ID is not a secret, and logging it lets the operator
		// add a legitimate Reacji Channeler to REACJI_BOT_IDS.
		app.logger.Printf("event: type=message event=%s result=rejected reason=untrusted_bot bot_id=%s", id, e.BotID)
		return
	}
	if !app.firstTime("message", rawID) {
		return
	}

	matched := matchCallbacks(app.callbacks, iconEmoji(e))
	if len(matched) == 0 {
		app.logger.Printf("event: type=message event=%s result=ignored reason=no_callback", id)
		return
	}

	channel := e.Channel
	for _, c := range matched {
		text := c.Message
		app.disp.submit(job{eventID: id, kind: "reacji", run: func(ctx context.Context) error {
			_, _, err := app.api.PostMessageContext(ctx, channel, slack.MsgOptionText(text, false))
			return err
		}}, app.reacjiRL)
	}
}
