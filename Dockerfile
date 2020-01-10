FROM golang:1-alpine AS gobuild
ENV GOPROXY https://proxy.golang.org
WORKDIR /work
COPY . .
RUN go build -o permbot ./cmd/permbot

FROM alpine:latest
COPY --from=gobuild /work/permbot /usr/local/bin/permbot
# Git is needed because this'll get called as a CI job from Gitlab
RUN apk -U add git
ENTRYPOINT ["/usr/local/bin/permbot"]
CMD ["permbot.toml"]
