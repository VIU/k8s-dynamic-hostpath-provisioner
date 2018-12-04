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
	"errors"
	"flag"
	"os"
	"path"
	"syscall"
	"fmt"

	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
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
	pvDir       string /* On-disk path of the PV root */
}


// NewHostPathProvisioner creates a new hostpath provisioner
func NewHostPathProvisioner(client kubernetes.Interface) controller.Provisioner {
	
	//No need for this, because it is assumed that directories
	//to be created are shared between worker nodes.
	/*
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		glog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
	}
	*/
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

	 /* Create the on-disk directory. */
	 path := path.Join(params.pvDir, options.PVName)
	 if err := os.MkdirAll(path, 0777); err != nil {
		 fmt.Printf("ERROR: failed to mkdir %s: %s", path, err)
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

	fmt.Printf("successfully created hostpath volume %s (%s)", options.PVName, path)

	return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations[provisionerIDAnn]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}

	/*
	 * Fetch the PV class to get the pvDir.  I don't think there would be
	 * any security implications from using the hostPath in the volume
	 * directly, but this feels more correct.
	 */	 
	class, err := p.client.StorageV1beta1().StorageClasses().Get(v1helper.GetPersistentVolumeClass(volume),
		metav1.GetOptions{})
	if err != nil {
		fmt.Printf("not removing volume <%s>: failed to fetch storageclass: %s",
			   volume.Name, err)
		return err
	}
	params, err := p.parseParameters(class.Parameters)
	if err != nil {
		fmt.Printf("not removing volume <%s>: failed to parse storageclass parameters: %s",
			   volume.Name, err)
		return err
	}

	/*
	 * Construct the on-disk path based on the pvDir and volume name, then
	 * delete it.
	 */
	 path := path.Join(params.pvDir, volume.Name)
	 if err := os.RemoveAll(path); err != nil {
		fmt.Printf("ERROR: failed to remove PV %s (%s): %v",
			 volume.Name, path, err)
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
 
		default:
			return nil, fmt.Errorf("invalid option %q", k)
		}
	}

	if params.pvDir == "" {
		return nil, fmt.Errorf("missing PV directory (pvDir)")
	}

	return &params, nil
}


func main() {
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")

	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("ERROR: failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("ERROR: to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		fmt.Printf("ERROR: error getting server version: %v", err)
	}
   
	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	hostPathProvisioner := NewHostPathProvisioner(clientset)

	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, hostPathProvisioner, serverVersion.GitVersion)
	pc.Run(wait.NeverStop)
}
