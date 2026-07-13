# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26.5-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tradedesk ./cmd/tradedesk

# ---- runtime stage (distroless, non-root, no shell) ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tradedesk /tradedesk
EXPOSE 8080
USER nonroot:nonroot
# distroless has no shell, so the container HEALTHCHECK runs the binary itself,
# which performs an internal HTTP GET against /healthz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/tradedesk", "--health"]
ENTRYPOINT ["/tradedesk"]
