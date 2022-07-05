module hiboma/reacjion

go 1.18

require (
	github.com/joho/godotenv v1.4.0
	github.com/slack-go/slack v0.11.0
	gopkg.in/yaml.v2 v2.4.0
)

require github.com/gorilla/websocket v1.5.0 // indirect

replace github.com/slack-go/slack => github.com/hiboma/slack v0.11.1-0.20220705071950-84c157492a03
