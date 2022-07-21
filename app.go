package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"gopkg.in/yaml.v2"
)

const REACJI_USERNAME = "Reacji Channeler"

type Callback struct {
	Emoji   string `yaml:"emoji"`
	Message string `yaml:"message"`
}

type App struct {
	callbacks []Callback
	client    *socketmode.Client
	Debug     bool
	logger    *log.Logger
}

func (app *App) Initalize() {
	app.logger = log.New(os.Stdout, "[reacjion  ] ", log.LstdFlags)
}

func (app *App) InitalizeCallbacks() {
	bytes, err := os.ReadFile("config.yaml")
	if err != nil {
		app.logger.Fatalf("failed to ReadFile config.yaml: %s", err)
	}
	yaml.Unmarshal(bytes, &app.callbacks)

	app.logger.Printf("initialized callbacks: %+v\n", app.callbacks)
}

func (app *App) InitializeSlack() {

	restClient := slack.New(
		os.Getenv("SLACK_BOT_TOKEN"),
		slack.OptionAppLevelToken(os.Getenv("SLACK_APP_TOKEN")),
		slack.OptionDebug(app.Debug),
		slack.OptionLog(log.New(os.Stderr, "[RESTclient] ", log.LstdFlags)),
	)

	app.client = socketmode.New(
		restClient,
		socketmode.OptionDebug(app.Debug),
		socketmode.OptionLog(log.New(os.Stderr, "[socketmode] ", log.LstdFlags)),
	)
}

func (app *App) RunLoop() {

	go func() {
		for event := range app.client.Events {

			if event.Type != socketmode.EventTypeEventsAPI {
				continue
			}

			eventAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
			if !ok {
				app.logger.Printf("event was ignored %v+n", eventAPIEvent)
				continue
			}
			app.client.Ack(*event.Request)

			switch eventAPIEvent.Type {
			case slackevents.CallbackEvent:
				innerEvent := eventAPIEvent.InnerEvent
				switch e := innerEvent.Data.(type) {
				case *slackevents.AppMentionEvent:
					app.handleMention(e)
				case *slackevents.MessageEvent:
					app.handleMessage(e)
				}
			}
		}
	}()

	app.client.Run()
}

func (app *App) handleMention(e *slackevents.AppMentionEvent) {
	var message string

	if strings.Contains(e.Text, "help") {
		message = "sorry, help is not prepared"
	} else {
		message = "pong"
	}

	app.client.PostMessage(e.Channel, slack.MsgOptionText(message, true))
}

func (app *App) handleMessage(e *slackevents.MessageEvent) {

	event, err := json.Marshal(e)

	// slackevents.MessageEven は、元々が JSON だったのを Unmarshal した内ものなので Marshal も成功するはず
	// Marshal が失敗するケースは特異っぽいので panic() で処理でいい
	if err != nil {
		panic(err)
	}

	if e.Username != REACJI_USERNAME {
		app.logger.Printf("message was ignored: %s\n", event)
		return
	}

	app.logger.Printf("message will handled: %s\n", event)

	for _, callback := range app.callbacks {
		if callback.Emoji == e.Icons.Emoji {

			app.logger.Printf("Emoji matched: %s\n", e.Icons.Emoji)
			_, _, err := app.client.PostMessage(
				e.Channel,
				slack.MsgOptionText(callback.Message, false),
			)

			if err != nil {
				app.logger.Printf("failed to PostMessage: %s", err)
				continue
			}
		}
	}
}
