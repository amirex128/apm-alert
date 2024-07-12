FROM ubuntu:latest

# Set the non-interactive mode for tzdata installation
ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /root

# Copy necessary files
COPY ./apm-alert /root/apm-alert
COPY ./.env /root/.env

# Install tzdata for time zone data
RUN apt-get update && \
    apt-get install -y tzdata && \
    rm -rf /var/lib/apt/lists/* && \
    chmod +x /root/apm-alert

CMD ["/root/apm-alert"]
