FROM golang:1.19 as builder
RUN apt-get -y update && apt-get -y install --no-install-recommends libopus-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app
# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -a -o soundboard ./...

FROM debian:11.5-slim
RUN apt-get -y update && apt-get -y install --no-install-recommends libopus0 ffmpeg && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/src/app/soundboard .

ENTRYPOINT [ "./soundboard" ]

EXPOSE 3000
