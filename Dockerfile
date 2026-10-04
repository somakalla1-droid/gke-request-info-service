# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/server
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/service"]
