FROM golang:latest

WORKDIR /app


COPY ./apm-alert /app/apm-alert
COPY ./.env /app/.env
COPY ./zoneinfo.zip /app/zoneinfo.zip
ENV ZONEINFO /app/zoneinfo.zip
ENV TZ=Asia/Tehran

RUN apt-get update && \
    apt-get install -y tzdata && \
    ln -fs /usr/share/zoneinfo/Asia/Tehran /etc/localtime && \
    echo "Asia/Tehran" > /etc/timezone && \
    dpkg-reconfigure --frontend noninteractive tzdata && \
    chmod +x /app/apm-alert

CMD ["/app/apm-alert"]
