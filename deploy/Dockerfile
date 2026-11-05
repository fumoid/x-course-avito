FROM golang:1.24-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /trip-service ./cmd/trip-service

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /trip-service /trip-service

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/trip-service"]
