FROM golang:1.25.0 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/diasoft-gateway ./cmd/diasoft-gateway

FROM gcr.io/distroless/base-debian12
COPY --from=build /bin/diasoft-gateway /diasoft-gateway
EXPOSE 8080
ENTRYPOINT ["/diasoft-gateway"]
