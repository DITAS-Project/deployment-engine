FROM golang:1.13-alpine as builder

RUN apk update
RUN apk add git

RUN mkdir $GOPATH/src/deployment-engine
COPY . $GOPATH/src/deployment-engine/
WORKDIR $GOPATH/src/deployment-engine

ENV GO111MODULE=on

RUN CGO_ENABLED=0 GOOS=linux go build -a -o /usr/bin/deployment-engine

FROM alpine:3.8

ENV BUILD_PACKAGES \
  openssh-client \
  sshpass \
  git \
  ansible

# Upgrading apk and system
RUN apk update && apk upgrade 

# Adding runtime packages
RUN apk add --no-cache ${BUILD_PACKAGES}

# Cleaning up
RUN rm -rf /var/cache/apk/*

# Adding hosts for convenience
RUN mkdir -p /etc/ansible /ansible
RUN echo "[local]" >> /etc/ansible/hosts
RUN echo "localhost" >> /etc/ansible/hosts

ENV ANSIBLE_GATHERING smart
ENV ANSIBLE_HOST_KEY_CHECKING false
ENV ANSIBLE_RETRY_FILES_ENABLED false
# For unknown reasons ansible might find a host unreachable but it can be reached.
ENV ANSIBLE_SSH_RETRIES 10
ENV ANSIBLE_SSH_PIPELINING True

COPY --from=builder /usr/bin/deployment-engine /usr/bin/deployment-engine

RUN mkdir /root/deployment-engine

WORKDIR /root/deployment-engine
COPY provision/ansible/kubernetes kubernetes
COPY provision/ansible/common common
COPY ditas/scripts ditas

RUN git clone https://github.com/DITAS-Project/VDC-Shared-Config.git

VOLUME /root
EXPOSE 8080
#Trying to run the app when the container starts
RUN ["chmod", "+x", "/usr/bin/deployment-engine"]
CMD ["deployment-engine"]

