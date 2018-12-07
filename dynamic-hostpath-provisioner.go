/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*

Original code: https://github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/tree/master/examples/hostpath-provisioner
and included code from https://github.com/torchbox/k8s-hostpath-provisioner/blob/master/hostpath-provisioner.go

*/

package main

import (
	"flag"
	"errors"
	"os"
	"path"
	"syscall"
	"fmt"
	"log"
	"io"
	"io/ioutil"

	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	provisionerName = "kazhar/dynamic-hostpath-provisioner"
	provisionerIDAnn = "kazhar/dynamic-hostpath-provisioner-id"
)

type hostPathProvisioner struct {

	client   kubernetes.Interface /* Kubernetes client for accessing the cluster during provision */

	// Identity of this hostPathProvisioner, set to node's name. Used to identify
	// "this" provisioner's PVs.
	identity string

}

 /* Storage the parsed configuration from the storage class */
 type hostPathParameters struct {
	pvDir           string /* On-disk path of the PV root */
}

/*
Logging configuration. 
Copied (directly, and without remorse) from: https://www.ardanlabs.com/blog/2013/11/using-log-package-in-go.html

TODO: JSON logging
https://github.com/sirupsen/logrus
https://github.com/francoispqt/onelog
https://github.com/uber-go/zap
https://github.com/rs/zerolog

*/
var (
    Trace   *log.Logger
    Info    *log.Logger
    Warning *log.Logger
    Error   *log.Logger
)

func InitLogger(
    traceHandle io.Writer,
    infoHandle io.Writer,
    warningHandle io.Writer,
    errorHandle io.Writer) {

    Trace = log.New(traceHandle,
        "TRACE: ", log.Lshortfile)

    Info = log.New(infoHandle,
        "INFO: ", log.Lshortfile)

    Warning = log.New(warningHandle,
        "WARNING: ", log.Lshortfile)

    Error = log.New(errorHandle,
        "ERROR: ", log.Lshortfile)
}


// NewHostPathProvisioner creates a new hostpath provisioner
func NewHostPathProvisioner(client kubernetes.Interface) controller.Provisioner {
	
	Info.Println("Creating NewHostPathProvisioner")

	return &hostPathProvisioner{
		client:    client,
		identity: provisionerIDAnn,
	}
}

var _ controller.Provisioner = &hostPathProvisioner{}

// Provision creates a storage asset and returns a PV object representing it.
func (p *hostPathProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

	/*
	* Fetch the PV root directory from the PV storage class.
	*/
	params, err := p.parseParameters(options.Parameters)
	if err != nil {
		return nil, err
	}

	Trace.Println("Provisioning directory...")
	Trace.Println("Options:",options)

	Trace.Println("PVName:",options.PVName)
	Trace.Println("PersistentVolumeReclaimPolicy when provisioning:",options.PersistentVolumeReclaimPolicy)
	Trace.Println("AccessModes:",options.PVC.Spec.AccessModes)

	 /* Create the on-disk directory. */
	 //TODO: check how to generate PVPath
	 //https://github.com/nmasse-itix/OpenShift-HostPath-Provisioner/blob/master/src/hostpath-provisioner/hostpath-provisioner.go
	 path := path.Join(params.pvDir, options.PVName)
	 if err := os.MkdirAll(path, 0777); err != nil {
		 //fmt.Printf("ERROR: failed to mkdir %s: %s", path, err)
		 Error.Println(fmt.Sprintf("failed to mkdir %s: %s", path, err))
		 return nil, err
	 }


	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
			Annotations: map[string]string{
				provisionerIDAnn: p.identity,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: path,
				},
			},
		},
	}

	Trace.Println(fmt.Sprintf("successfully created hostpath volume %s (%s)", options.PVName, path))

	return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
// Does not delete if ReclaimPolicy is set to "Retain" in StorageClass configuration.
func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {

	Trace.Println("Deleting directory...")

	//check that this volume was provisioned by this provisioner
	ann, ok := volume.Annotations[provisionerIDAnn]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}

	Trace.Println("Volume: ",volume)
	Trace.Println("hostpathSource: ",volume.Spec.HostPath)

	//get host pathPV volume spec
	path := volume.Spec.HostPath.Path

	Trace.Println("path: ",path)	
	
	//get reclaim policy of this volume
	volumeClaimPolicy := volume.Spec.PersistentVolumeReclaimPolicy
	Trace.Println("volumeClaimPolicy: ",volumeClaimPolicy)
	
	if volumeClaimPolicy != "Delete" {
		Error.Println("Will not delete directory. PersistentVolumeReclaimPolicy is not Delete. It is: ", volumeClaimPolicy)
		return &controller.IgnoredError{Reason: "PersistentVolumeReclaimPolicy was not Delete. No action taken."}
	}

	 if err := os.RemoveAll(path); err != nil {
		Error.Println(fmt.Sprintf("Remove dir (%s) failed:  %s",volume.Name, err))
		return err
	 }
 
	return nil
}

/*
* Parse parameters given in StorageClass
*/
func (p *hostPathProvisioner) parseParameters(parameters map[string]string) (*hostPathParameters, error) {
	var params hostPathParameters
	
	for k, v := range parameters {
		switch k {
		case "pvDir":
			params.pvDir = v
			
		case "enableTrace":
			if v == "true" {
				InitLogger(os.Stdout, os.Stdout, os.Stdout, os.Stderr)
			} else {
				InitLogger(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
			}		
		default:
			return nil, fmt.Errorf("invalid option %q", k)
		}
	}

	Trace.Println("storageclass parameters: ",parameters)


	if params.pvDir == "" {
		return nil, fmt.Errorf("missing PV directory (pvDir)")
	}

	return &params, nil
}

func main() {
	syscall.Umask(0)
	flag.Parse()
	flag.Set("logtostderr", "true")

	// initialize logger
	InitLogger(os.Stdout, os.Stdout, os.Stdout, os.Stderr)

	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		Error.Println(fmt.Sprintf("ERROR: failed to create config: %v", err))
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		Error.Println(fmt.Sprintf("ERROR: to create client: %v", err))
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		Error.Println(fmt.Sprintf("ERROR: error getting server version: %v", err))
	}
   
	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	hostPathProvisioner := NewHostPathProvisioner(clientset)

	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, hostPathProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
}
