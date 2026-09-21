# Build the go application into a binary
FROM golang:alpine AS builder
RUN apk --update add ca-certificates
WORKDIR /app
COPY . ./
RUN go mod tidy -diff
# Shown by `go-uptime version`
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-s -w -X 'github.com/jniltinho/go-uptime/v7/cmd.Version=${VERSION}' -X 'github.com/jniltinho/go-uptime/v7/cmd.GitCommit=${GIT_COMMIT}' -X 'github.com/jniltinho/go-uptime/v7/cmd.BuildDate=${BUILD_DATE}'" \
    -o go-uptime .
# /gatus stays as a symbolic link during the 7.x series, for the compose files of v6 that call the binary by its path.
# It is created inside a directory: copying the directory keeps the link, copying the link alone would resolve it.
RUN mkdir /compat && ln -s /go-uptime /compat/gatus

# Run Tests inside docker image if you don't have a configured go environment
#RUN apk update && apk add --virtual build-dependencies build-base gcc
#RUN go test ./... -mod vendor

# Run the binary on an empty container
FROM scratch
COPY --from=builder /app/go-uptime /go-uptime
COPY --from=builder /compat/ /
COPY --from=builder /app/config.yaml /config/config.yaml
COPY --from=builder /app/LICENSE /app/NOTICE /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# No ENV for GO_UPTIME_CONFIG_PATH nor GO_UPTIME_LOG_LEVEL: the application has the defaults, and a default of the image
# would always be set, hiding the GATUS_* variables of a compose file of v6
ENV PORT="8080"
EXPOSE ${PORT}
# The image has no shell, so the binary checks itself: the address, the port and the TLS come from the configuration
HEALTHCHECK --interval=30s --timeout=6s --start-period=30s --retries=3 CMD ["/go-uptime", "healthcheck"]
ENTRYPOINT ["/go-uptime"]
