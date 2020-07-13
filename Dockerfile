FROM ubuntu:bionic
COPY ./build/linux/amd64/reflex /reflex
WORKDIR /
ENTRYPOINT [ "/reflex" ]