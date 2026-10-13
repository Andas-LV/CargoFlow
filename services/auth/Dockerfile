# syntax=docker/dockerfile:1
# Build context: repository root.
#   docker build -f services/auth/Dockerfile -t cargoflow-auth .

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY gen ./gen
COPY pkg ./pkg
COPY services/auth ./services/auth
# Only modules needed by this binary are downloaded (not the dev tools listed in go.mod).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./services/auth/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 50051 8081
ENTRYPOINT ["/app"]
