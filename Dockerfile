FROM golang:1.20-bullseye

RUN apt update && apt upgrade -y && apt install -y git make sudo python3-pip cron
RUN pip3 install rembg

# you should fix this path according you have
COPY ./.data/u2net.onnx /data/u2net.onnx

ENV U2NET_HOME /data

WORKDIR /app

COPY . .

ENTRYPOINT [ "/app/docker-entrypoint.sh" ]
