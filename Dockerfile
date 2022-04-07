FROM gcr.io/distroless/static
USER nonroot:nonroot
COPY --chown=nonroot:nonroot bin/traderbot /traderbot
ENTRYPOINT ["/traderbot"]
