

# https://hub.docker.com/_/golang/tags
FROM golang:1.23.2 AS build
ARG TARGETARCH
WORKDIR /root/
RUN mkdir -p /root/tgzebot/

RUN apt update
RUN apt -y -q install xz-utils

RUN curl -s -S -L -O https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-$TARGETARCH-static.tar.xz
RUN ls -l -a
RUN tar -x -J -f ffmpeg-release-$TARGETARCH-static.tar.xz
RUN ls -l -a
RUN mv /root/ffmpeg-*-static/ffmpeg /root/tgzebot/ffmpeg
RUN /root/tgzebot/ffmpeg -version

COPY tgzebot.go go.mod go.sum /root/tgzebot/
WORKDIR /root/tgzebot/
RUN go version
RUN go get -a -v
RUN ls -l -a
RUN go build -o tgzebot tgzebot.go
RUN ls -l -a



# https://hub.docker.com/_/alpine/tags
FROM alpine:3.20.3
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2

COPY --from=ghcr.io/shoce/tgbotserver:24.1104.0404 /bin/tgbotserver /bin/tgbotserver
COPY --from=build /root/tgzebot/ffmpeg /bin/ffmpeg
COPY --from=build /root/tgzebot/tgzebot /bin/tgzebot
RUN ls -l -a /bin/tgbotserver /bin/ffmpeg /bin/tgzebot

WORKDIR /root/
ENTRYPOINT ["/bin/tgzebot"]


