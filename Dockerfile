# Build the go application into a binary
FROM golang:alpine AS builder
RUN apk --update add ca-certificates
WORKDIR /app
COPY . ./
RUN go mod tidy -diff
# Shown by `gatus version`
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-s -w -X 'github.com/jniltinho/go-uptime/v7/cmd.Version=${VERSION}' -X 'github.com/jniltinho/go-uptime/v7/cmd.GitCommit=${GIT_COMMIT}' -X 'github.com/jniltinho/go-uptime/v7/cmd.BuildDate=${BUILD_DATE}'" \
    -o gatus .

# Run Tests inside docker image if you don't have a configured go environment
#RUN apk update && apk add --virtual build-dependencies build-base gcc
#RUN go test ./... -mod vendor

# Run the binary on an empty container
FROM scratch
COPY --from=builder /app/gatus .
COPY --from=builder /app/config.yaml ./config/config.yaml
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV GATUS_CONFIG_PATH=""
ENV GATUS_LOG_LEVEL="INFO"
ENV PORT="8080"
EXPOSE ${PORT}
# The image has no shell, so the binary checks itself: the address, the port and the TLS come from the configuration
HEALTHCHECK --interval=30s --timeout=6s --start-period=30s --retries=3 CMD ["/gatus", "healthcheck"]
ENTRYPOINT ["/gatus"]
