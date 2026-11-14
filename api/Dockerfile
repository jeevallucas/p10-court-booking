# The API and the seed program in one small image, for docker compose and the demo host.
FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# No shell or package manager; the binaries carry their own time zone data.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /out/seed /
EXPOSE 8080
ENTRYPOINT ["/api"]
