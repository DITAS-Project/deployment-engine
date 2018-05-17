# Dockerfile for Python, Go and Ansible with the deployment engine, MySQL in a parallel container.
# WIP = work in progress
FROM alpine:3.7
 
ENV ANSIBLE_VERSION 2.5.0
 
ENV BUILD_PACKAGES \
  bash \
  curl \
  tar \
  openssh-client \
  sshpass \
  git \
  python \
  py-boto \
  py-dateutil \
  py-httplib2 \
  py-jinja2 \
  py-paramiko \
  py-pip \
  py-yaml \
  go>=1.9.4-r0 \
  gcc>=6.3.0-r4 \
  g++>=6.3.0-r4 \
  ca-certificates
 
RUN set -x && \
    \
    echo "==> Adding build-dependencies..."  && \
    apk --update add --virtual build-dependencies \
      gcc \
      musl-dev \
      libffi-dev \
      python-dev && \
    \
    echo "==> Upgrading apk and system..."  && \
    apk update && apk upgrade && \
    \
    echo "==> Adding Python runtime and packages..."  && \
    apk add --no-cache ${BUILD_PACKAGES} && \
    pip install --upgrade pip && \
    pip install python-keyczar docker-py && \
    \
    echo "==> Adding Cloudsigma Python api..."  && \
    pip install cloudsigma && \
    \
    echo "==> Installing Ansible..."  && \
    pip install ansible==${ANSIBLE_VERSION} && \
    \
    echo "==> Installing MySQLdb..."  && \
    apk add --no-cache --virtual .build-deps mariadb-dev && \
    pip install MySQL-python &&\
    apk add --virtual .runtime-deps mariadb-client-libs && \
    apk del .build-deps && \
    \
    echo "==> Getting GO packages..."  && \
    go get github.com/go-sql-driver/mysql && \
    go get github.com/gorilla/mux && \
    \
    echo "==> Cleaning up..."  && \
    apk del build-dependencies && \
    rm -rf /var/cache/apk/* && \
    \
    echo "==> Adding hosts for convenience..."  && \
    mkdir -p /etc/ansible /ansible && \
    echo "[local]" >> /etc/ansible/hosts && \
    echo "localhost" >> /etc/ansible/hosts
 
ENV ANSIBLE_GATHERING smart
ENV ANSIBLE_HOST_KEY_CHECKING false
ENV ANSIBLE_RETRY_FILES_ENABLED false
ENV ANSIBLE_ROLES_PATH /ansible/playbooks/roles
ENV ANSIBLE_SSH_PIPELINING True
ENV PYTHONPATH /ansible/lib
ENV PATH /ansible/bin:$PATH
ENV ANSIBLE_LIBRARY /ansible/library
 
WORKDIR /deployment-engine/src
COPY .cloudsigma.conf /root/.cloudsigma.conf
COPY /src /deployment-engine/src
RUN ssh-keygen -q -t rsa -N '' -f /root/.ssh/id_rsa
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o src .
EXPOSE 8080
