FROM kazhar/k8s-dynamic-hostpath-provisioner:build as build-stage 
#FROM myhostprovisioner:build as build-stage 

WORKDIR /go/src/k8s-dynamic-hostpath-provisioner

#copy and build the code
COPY dynamic-hostpath-provisioner.go .
RUN make dynamic-hostpath-provisioner

#CMD ["/bin/bash"]

FROM scratch

COPY --from=build-stage /go/src/k8s-dynamic-hostpath-provisioner/dynamic-hostpath-provisioner /
CMD ["/dynamic-hostpath-provisioner"]
