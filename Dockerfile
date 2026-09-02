# ---- build stage ----
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 works because modernc.org/sqlite is pure Go.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /carwash-api .

# ---- runtime stage ----
FROM alpine:3.20
RUN adduser -D -h /app app && mkdir -p /app/data && chown app:app /app/data
WORKDIR /app
COPY --from=build /carwash-api /usr/local/bin/carwash-api

USER app
ENV PORT=8080 \
    DB_PATH=/app/data/carwash.db
VOLUME ["/app/data"]
EXPOSE 8080

CMD ["carwash-api"]
