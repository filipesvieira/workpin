FROM golang:1.22-alpine AS build
WORKDIR /src
COPY services/api/ ./
RUN CGO_ENABLED=0 go build -o /api ./cmd/api
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
COPY --from=build /api /usr/local/bin/api
USER app
EXPOSE 8080
CMD ["api"]
