FROM ubuntu:22.10

RUN echo 'hosts: files dns' >> /etc/nsswitch.conf
RUN apk add --no-cache iputils ca-certificates net-snmp-tools procps lm_sensors tzdata su-exec libcap wget tar libc6-compat file && \
    update-ca-certificates

RUN set -ex && \
    mkdir -p /usr/bin /etc/categraf 

COPY conf /etc/categraf/conf
COPY categraf  /usr/bin/categraf

COPY entrypoint.sh /entrypoint.sh
CMD ["/entrypoint.sh"]
