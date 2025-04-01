FROM gcr.io/distroless/static:nonroot
USER 65532:65532

COPY kpm /usr/local/bin/kpm
ENTRYPOINT ["/usr/local/bin/kpm"]