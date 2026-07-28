FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY *.go ./
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /slack-bridge .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /slack-bridge /slack-bridge
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/slack-bridge"]
