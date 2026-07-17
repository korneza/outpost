FROM gcr.io/distroless/static-debian12:nonroot
COPY outpost /usr/local/bin/outpost
ENTRYPOINT ["/usr/local/bin/outpost"]
CMD ["serve"]
