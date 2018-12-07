FROM golang:1.11.2 as build-stage 

# install dep
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

# build
WORKDIR /go/src/k8s-dynamic-hostpath-provisioner
#use cached file to get dependencies, useful during development when this is in local Docker cache
COPY cache/dynamic-hostpath-provisioner.go .
COPY ./Makefile .
RUN	make dep

#copy and build actual code
COPY dynamic-hostpath-provisioner.go .
RUN make dynamic-hostpath-provisioner

#CMD ["/bin/bash"]

FROM scratch
COPY --from=build-stage /go/src/k8s-dynamic-hostpath-provisioner/dynamic-hostpath-provisioner /
CMD ["/dynamic-hostpath-provisioner"]
 