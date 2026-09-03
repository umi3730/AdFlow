FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/adflow ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/adflow /adflow
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/adflow"]
