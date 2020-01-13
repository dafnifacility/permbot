ifeq ($(CI_COMMIT_TAG),)
# Use the branch name if the tag isn't available
GIT_DESCRIBE := $(shell git describe --always --dirty)
else
GIT_DESCRIBE := $(CI_COMMIT_TAG)
endif

PKGS := $(shell go list ./...)

.PHONY: permbot
permbot:
	go build -ldflags="-X gitlab.dafni.rl.ac.uk/dafni/tools/permbot/internal/app/permbot.PermbotVersion=$(GIT_DESCRIBE)" -o permbot ./cmd/permbot

.PHONY: test coverage
test:
	go test $(PKGS)

cover.out: test
	go test $(PKGS) -coverprofile cover.out

coverage: cover.out
	go tool cover -func=cover.out

clean:
	rm -vf permbot cover.out
