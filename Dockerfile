# Build stage.
FROM golang:1.25.0-alpine AS builder

WORKDIR /src

# Dependencies first, so a source-only change does not re-download the module
# cache on every build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

# CGO_ENABLED=0 produces a static binary, which is what lets the final stage be
# a scratch-class image with no libc.
#
# -s -w strips the symbol table and DWARF debug info. It costs roughly 25% of
# the binary size and, more usefully, keeps internal symbol names out of an
# image that ships to production. Panic stack traces stay intact — only the
# debugger metadata goes.
#
# -trimpath removes the build machine's absolute paths, which would otherwise
# embed the builder's directory layout in the binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w -X github.com/sujanto-gaws/kopiochi/internal/version.Version=${VERSION}" \
        -o /out/kopiochi ./cmd/api

# The migrator is a separate binary on purpose: goose has no business linking
# into the server (repository-hygiene.md, problem 5). It ships in the same
# image so the compose stack can run it as a one-shot service without a second
# build.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w -X github.com/sujanto-gaws/kopiochi/internal/version.Version=${VERSION}" \
        -o /out/kopiochi-migrate ./cmd/migrate

# Runtime stage.
#
# distroless/static rather than alpine:latest. Two reasons, in order of
# importance: there is no shell, no package manager and no busybox, so there is
# nothing to pivot to if the process is compromised; and `alpine:latest` is a
# moving tag, so two builds a week apart could ship different base images with
# no diff to show for it.
#
# It carries ca-certificates and tzdata, which is everything a static Go binary
# needs — the apk install the previous version did was for exactly those two.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# :nonroot runs as uid 65532. The previous image ran as root in /root/, which
# makes "container escape" and "root on the host" a much shorter chain than it
# needs to be.
COPY --from=builder --chown=nonroot:nonroot /out/kopiochi /app/kopiochi
COPY --from=builder --chown=nonroot:nonroot /out/kopiochi-migrate /app/kopiochi-migrate
COPY --chown=nonroot:nonroot config/default.yaml /app/config/default.yaml

# Deliberately NOT copied: keys/, .env, and everything else .dockerignore
# excludes. Signing keys belong in a mounted secret or a secret store, never
# baked into a layer that is pushed to a registry and cached forever.

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/kopiochi"]
CMD ["serve", "--config", "config/default.yaml"]
