FROM golang:1.26-alpine3.23 AS build
RUN apk add --no-cache build-base pkgconf vips-dev ffmpeg
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=1 go build -tags libvips -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker
RUN CGO_ENABLED=1 go test -tags libvips ./internal/worker

FROM alpine:3.23
RUN apk add --no-cache ca-certificates vips ffmpeg && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
RUN mkdir -p /app/data && chown app:app /app/data
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
