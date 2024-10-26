
# https://hub.docker.com/_/golang/tags
FROM golang:1.23.2 AS build
ARG TARGETARCH
WORKDIR /root/
RUN apt update
RUN apt -y -q install xz-utils
RUN curl -s -S -L -O https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-$TARGETARCH-static.tar.xz
RUN ls -l -a
RUN tar -x -J -f ffmpeg-release-$TARGETARCH-static.tar.xz
RUN ls -l -a
RUN mkdir -p /root/tgzebot/
COPY tgzebot.go go.mod go.sum /root/tgzebot/
RUN mv /root/ffmpeg-*-static/ffmpeg /root/tgzebot/ffmpeg
RUN /root/tgzebot/ffmpeg -version
WORKDIR /root/tgzebot/
RUN go version
RUN go get -a -u -v
RUN ls -l -a
RUN go build -o tgzebot tgzebot.go
RUN ls -l -a


# https://hub.docker.com/_/alpine/tags
FROM alpine:3.20.3
RUN apk add --no-cache tzdata
RUN apk add --no-cache gcompat && ln -s -f -v ld-linux-x86-64.so.2 /lib/libresolv.so.2
RUN mkdir -p /opt/tgzebot/
COPY --from=build /root/tgzebot/ffmpeg /opt/tgzebot/ffmpeg
COPY --from=build /root/tgzebot/tgzebot /opt/tgzebot/tgzebot
RUN ls -l -a /opt/tgzebot/
WORKDIR /opt/tgzebot/
ENTRYPOINT ["./tgzebot"]


