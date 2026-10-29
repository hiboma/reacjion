module hiboma/reacjion

go 1.27

require (
	github.com/joho/godotenv v1.5.1
	github.com/slack-go/slack v0.11.0
	gopkg.in/yaml.v2 v2.4.0
)

require github.com/gorilla/websocket v1.5.3 // indirect

replace github.com/slack-go/slack => github.com/hiboma/slack v0.11.1-0.20220705071950-84c157492a03
