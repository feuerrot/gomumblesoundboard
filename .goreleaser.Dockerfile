FROM debian:11.5-slim
RUN apt-get -y update && apt-get -y install --no-install-recommends libopus0 ffmpeg && rm -rf /var/lib/apt/lists/*

COPY soundboard /soundboard
ENTRYPOINT [ "/soundboard" ]

EXPOSE 3000
