FROM golang:1-alpine AS gobuild
ENV GOPROXY https://proxy.golang.org
WORKDIR /work
COPY . .
RUN apk -U add git make && make permbot permbot_agent

FROM alpine:latest
COPY --from=gobuild /work/permbot /usr/local/bin/permbot
COPY --from=gobuild /work/permbot_agent /usr/local/bin/permbot_agent
# Git is needed because this'll get called as a CI job from Gitlab
RUN apk -U add git
ENTRYPOINT ["/usr/local/bin/permbot"]
CMD ["permbot.toml"]
