# FILE IS AUTOMATICALLY MANAGED BY github.com/vegaprotocol/terraform//github
FROM gcr.io/distroless/static
USER nonroot:nonroot
COPY --chown=nonroot:nonroot bin/liqbot /liqbot
ENTRYPOINT ["/liqbot"]
