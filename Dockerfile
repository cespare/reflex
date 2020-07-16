FROM ubuntu:bionic
COPY ./build/linux/amd64/reflex /reflex
RUN apt-get update && apt-get install -y curl
WORKDIR /
ENTRYPOINT [ "/reflex" ]