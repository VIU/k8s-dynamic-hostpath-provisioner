FROM golang:1.8 as build-stage 

# build
WORKDIR /go/src/k8s-dynamic-hostpath-provisioner

#COPY ./glide* ./
COPY ./vendor ./vendor/
COPY ./dynamic-hostpath-provisioner.go .

#build code
RUN CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o dynamic-hostpath-provisioner .

#CMD ["/bin/bash"]

FROM scratch
COPY --from=build-stage /go/src/k8s-dynamic-hostpath-provisioner/dynamic-hostpath-provisioner /
CMD ["/dynamic-hostpath-provisioner"]
 