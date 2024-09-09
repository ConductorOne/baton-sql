FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-sql"]
COPY baton-sql /