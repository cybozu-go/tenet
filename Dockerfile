FROM scratch
LABEL org.opencontainers.image.authors="Cybozu, Inc." \
      org.opencontainers.image.title="tenet" \
      org.opencontainers.image.source="https://github.com/cybozu-go/tenet"
WORKDIR /
COPY LICENSE /
COPY tenet /
USER 65532:65532

ENTRYPOINT ["/tenet"]
