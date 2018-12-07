FROM golang:1.11.2

#build image to include dependencies

# install dep
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

# build
WORKDIR /go/src/k8s-dynamic-hostpath-provisioner

COPY dynamic-hostpath-provisioner.go .
COPY ./Makefile .

RUN	make dep

CMD ["/bin/bash"]
 