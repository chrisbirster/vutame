FROM node:24-alpine AS web
WORKDIR /src
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build:web

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY go.mod ./
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vutame ./cmd/vutame

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/vutame /vutame
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/vutame"]
