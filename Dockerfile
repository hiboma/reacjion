FROM golang:1.27.3 AS builder

WORKDIR /go/src/github.com/hiboma/reacjion/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux make build

FROM alpine:3.24.3
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /go/src/github.com/hiboma/reacjion/reacjion ./

CMD ["./reacjion"]
