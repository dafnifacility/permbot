FROM golang:1-alpine AS gobuild
ENV GOPROXY https://proxy.golang.org
WORKDIR /work
COPY . .
RUN go build -o permbot ./cmd/permbot

FROM alpine:latest
COPY --from=gobuild /work/permbot /usr/local/bin/permbot
ENTRYPOINT ["/usr/local/bin/permbot"]
CMD ["permbot.toml"]
