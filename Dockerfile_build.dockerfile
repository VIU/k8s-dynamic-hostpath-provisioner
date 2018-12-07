FROM golang:1.11.2

#build image to include dependencies

#install dep
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

#make for dependencies
WORKDIR /go/src/k8s-dynamic-hostpath-provisioner
#must use source in build dir because it includes dependencies that are needed in the code, 
#even if the code does not require those. If using only the code, 
#build fails with messages:
#k8s-dynamic-hostpath-provisioner/vendor/sigs.k8s.io/yaml 
#vendor/sigs.k8s.io/yaml/yaml.go:51:17: undefined: yaml.UnmarshalStrict 
#vendor/sigs.k8s.io/yaml/yaml.go:118:28: undefined: yaml.UnmarshalStrict
COPY build/dynamic-hostpath-provisioner.go .
COPY ./Makefile .
RUN	make dep

CMD ["/bin/bash"]
 