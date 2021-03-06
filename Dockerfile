FROM docker.01-edu.org/golang:1.16.3-alpine3.13 as builder

ENV GIT_TERMINAL_PROMPT=0
RUN apk add --no-cache git

WORKDIR /app
COPY go.* ./
RUN go mod download
COPY cmd cmd
COPY *.go ./
RUN go build ./cmd/runner

FROM docker.01-edu.org/alpine:3.13.4

RUN apk add --no-cache tzdata

ENTRYPOINT ["/app/runner"]

COPY --from=builder /app/runner /app/runner
