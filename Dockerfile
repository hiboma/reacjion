FROM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS builder

WORKDIR /go/src/github.com/hiboma/reacjion/
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o reacjion .

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
# The pinned Alpine image already includes ca-certificates-bundle.
RUN test -s /etc/ssl/certs/ca-certificates.crt \
 && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=builder /go/src/github.com/hiboma/reacjion/reacjion ./

USER 10001
CMD ["./reacjion"]
