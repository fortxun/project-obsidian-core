FROM otel/opentelemetry-collector-contrib:0.96.0 as otelcol

FROM alpine:3.19

COPY --from=otelcol /otelcol-contrib /otelcol
COPY config/otel-config.yaml /etc/otel-collector/otel-config.yaml

# Create non-root user
RUN addgroup -g 10001 otel && \
    adduser -D -u 10001 -G otel otel

USER otel

EXPOSE 4317 4318
VOLUME ["/etc/otel-collector"]
ENTRYPOINT ["/otelcol"]
CMD ["--config=/etc/otel-collector/otel-config.yaml"]