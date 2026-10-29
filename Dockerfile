FROM golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS builder

WORKDIR /go/src/github.com/hiboma/reacjion/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux make build

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk --no-cache add ca-certificates \
 && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=builder /go/src/github.com/hiboma/reacjion/reacjion ./

USER 10001
CMD ["./reacjion"]
