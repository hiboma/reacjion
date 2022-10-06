FROM golang:1.19.2

WORKDIR /go/src/github.com/hiboma/reacjion/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux make build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=0 /go/src/github.com/hiboma/reacjion/reacjion ./

CMD ["./reacjion"]
