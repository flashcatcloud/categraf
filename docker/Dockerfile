FROM ubuntu:24.04

RUN echo 'hosts: files dns' >> /etc/nsswitch.conf

RUN set -ex && \
    mkdir -p /usr/bin /etc/categraf 

COPY categraf  /usr/bin/categraf

COPY conf /etc/categraf/conf

COPY entrypoint.sh /entrypoint.sh

CMD ["/entrypoint.sh"]
