FROM golang:1.25.8 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/diasoft-gateway ./cmd/diasoft-gateway

FROM gcr.io/distroless/base-debian12
COPY --from=build /bin/diasoft-gateway /diasoft-gateway
COPY api/openapi/openapi.yaml /srv/openapi.yaml
COPY api/openapi/swagger.html /srv/swagger.html
COPY api/openapi/swagger-ui /srv/swagger-ui
EXPOSE 8080
ENTRYPOINT ["/diasoft-gateway"]
