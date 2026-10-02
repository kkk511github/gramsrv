FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ca-certificates tzdata ffmpeg openssl \
    && addgroup -S -g 10001 safelink \
    && adduser -S -D -H -u 10001 -G safelink safelink
COPY --chmod=0555 safelink-server /app/safelink-server
COPY langpack/ /app/langpack/
USER 10001:10001
WORKDIR /app
ENTRYPOINT ["/app/safelink-server"]
