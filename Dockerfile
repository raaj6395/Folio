FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/folio ./cmd/folio

FROM alpine:3.22 AS typst
ARG TYPST_VERSION=0.15.1
RUN apk add --no-cache curl xz \
 && curl -fsSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" \
  | tar -xJ --strip-components=1 -C /usr/local/bin "typst-x86_64-unknown-linux-musl/typst"

FROM alpine:3.22
RUN apk add --no-cache ca-certificates fontconfig \
 && adduser -D -u 10001 folio
COPY --from=typst /usr/local/bin/typst /usr/local/bin/typst
COPY --from=build /out/folio /usr/local/bin/folio
USER folio
ENV FOLIO_HOME=/tmp/folio
EXPOSE 10000
CMD ["folio", "http", "--unsafe-public"]
