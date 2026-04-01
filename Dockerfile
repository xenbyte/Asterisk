FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot
RUN mkdir -p /src/data

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /bot /bot
COPY --from=builder --chown=nonroot:nonroot /src/data /data
USER nonroot:nonroot
ENTRYPOINT ["/bot"]
