/* Original copyright and license:
 *
 * Copyright 2016 The Kubernetes Authors.
 * Copyright 2017 Torchbox Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

 package main

 import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	//"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	//"github.com/pkg/xattr"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/api/v1/helper"
	"syscall"
 )
 
 /* Our constants */
 const (
	 resyncPeriod     = 15 * time.Second
	 provisionerName  = "kazhar/dynamic-hostpath"
	 provisionerIDAnn = "kazhar/dynamic-hostpath-provisioner-id"
 )
 
 /* Our provisioner class, which implements the controller API. */
 type hostPathProvisioner struct {
	 client   kubernetes.Interface /* Kubernetes client for accessing the cluster during provision */
	 identity string               /* Our unique provisioner identity */
 }
 
 /* Storage the parsed configuration from the storage class */
 type hostPathParameters struct {
	 pvDir       string /* On-disk path of the PV root */
 }
 
 /*
  * Create a new provisioner from a given client and identity.
  */
 func NewHostPathProvisioner(client kubernetes.Interface, id string) controller.Provisioner {
	 return &hostPathProvisioner{
		 client:   client,
		 identity: id,
	 }
 }
 
 var _ controller.Provisioner = &hostPathProvisioner{}
 
 /*
 * Generate PV path. Copied from https://github.com/nmasse-itix/OpenShift-HostPath-Provisioner/blob/master/src/hostpath-provisioner/hostpath-provisioner.go
 */
 func (p *hostPathProvisioner) generatePVPath(options controller.VolumeOptions) (string, error) {

	params, err := p.parseParameters(options.Parameters)
	if err != nil {
		return "", err
	}

	// Default value for Name Generation
	namespace := "_"
	name := options.PVName

	// Try to get information from PVC
	if pvc := options.PVC; pvc != nil {
		// Get PVC namespace if it exists
		ns := pvc.Namespace
		if ns != "" {
			namespace = ns
		}

		// Get PVC name if it exists
		n := pvc.Name
		if n != "" {
			name = n
		}
	}

	// Try to create namespace dir if it does not exist
	nspath := path.Join(params.pvDir, namespace)
	if _, err := os.Stat(nspath); os.IsNotExist(err) {
		if err := os.MkdirAll(nspath, 0777); err != nil {
			return "", err
		}
	}

	// Check if pvc name already exists
	pvpath := path.Join(nspath, name)
	if _, err := os.Stat(pvpath); err == nil {
		// If yes, try to generate a new name
		for i := 1; i < 100; i++ {
			 new_name := fmt.Sprintf("%s-%02d", name, i)
			 new_pvpath := path.Join(nspath, new_name)
			 if _, err := os.Stat(new_pvpath); os.IsNotExist(err) {
				 // Found a free name
				 name = new_name
				 pvpath = new_pvpath
				 return pvpath, nil
			 }
		}
	}

	return pvpath, nil
}

 /*
  * Provision: create the physical on-disk path for this PV and return a new
  * volume referencing it as a hostPath.  The volume is annotated with our
  * provisioner id, so multiple provisioners can run on the same cluster.
  */
 func (p *hostPathProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	 /*
	  * Fetch the PV root directory from the PV storage class.
	  */
	/* params, err := p.parseParameters(options.Parameters)
	 if err != nil {
		 return nil, err
	 }
 */
	 /*
	  * Extract the PV capacity as bytes. 
	  */
	 capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	 volbytes := capacity.Value()
	 glog.Infof("pv storage: %+v", volbytes)
 
	 if volbytes <= 0 {
		 return nil, fmt.Errorf("storage capacity must be >= 0 (not %+v)", capacity.String())
	 }
 
	 path, err := p.generatePVPath(options)
	 if err != nil {
		 return nil, err
	 }	

	 /* Create the on-disk directory. */
	 if err := os.MkdirAll(path, 0777); err != nil {
		glog.Errorf("failed to mkdir %s: %s", path, err)
		return nil, err
		}	
/*
	 path := path.Join(params.pvDir, options.PVName)
	 if err := os.MkdirAll(path, 0777); err != nil {
		 glog.Errorf("failed to mkdir %s: %s", path, err)
		 return nil, err
	 }
 */
	 /* The actual PV we will create */
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
 
	 glog.Infof("successfully created hostpath volume %s (%s)",
		 options.PVName, path)
 
	 return pv, nil
 }
 
 /*
  * Delete: remove a PV from the disk by deleting its directory.
  */
 func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {
	 /* Ensure this volume was provisioned by us */
	 ann, ok := volume.Annotations[provisionerIDAnn]
	 if !ok {
		 glog.Infof("not removing volume <%s>: identity annotation <%s> missing",
				volume.Name, provisionerIDAnn)
		 return errors.New("identity annotation not found on PV")
	 }
 
	 if ann != p.identity {
		 glog.Infof("not removing volume <%s>: identity annotation <%s> does not match ours <%s>",
				volume.Name, p.identity, provisionerIDAnn)
		 return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	 }
 
	 /*
	  * Fetch the PV class to get the pvDir.  I don't think there would be
	  * any security implications from using the hostPath in the volume
	  * directly, but this feels more correct.
	  */
	 class, err := p.client.StorageV1beta1().StorageClasses().Get(
		 helper.GetPersistentVolumeClass(volume),
		 metav1.GetOptions{})
	 if err != nil {
		 glog.Infof("not removing volume <%s>: failed to fetch storageclass: %s",
				volume.Name, err)
		 return err
	 }
 
	 params, err := p.parseParameters(class.Parameters)
	 if err != nil {
		 glog.Infof("not removing volume <%s>: failed to parse storageclass parameters: %s",
				volume.Name, err)
		 return err
	 }
 
	 /*
	  * Construct the on-disk path based on the pvDir and volume name, then
	  * delete it.
	  */
	 path := path.Join(params.pvDir, volume.Name)
	 if err := os.RemoveAll(path); err != nil {
		 glog.Errorf("failed to remove PV %s (%s): %v",
			 volume.Name, path, err)
		 return err
	 }
 
	 return nil
 }
 
 func (p *hostPathProvisioner) parseParameters(parameters map[string]string) (*hostPathParameters, error) {
	 var params hostPathParameters
 
	 for k, v := range parameters {
		 switch strings.ToLower(k) {
		 case "pvdir":
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
 
 var (
	 master     = flag.String("master", "", "Master URL")
	 kubeconfig = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
	 name       = flag.String("name", "", "Provisioner name")
	 id         = flag.String("id", "", "Unique provisioner identity")
 )
 
 func main() {
	 syscall.Umask(022)
 
	 flag.Parse()
	 flag.Set("logtostderr", "true")
 
	 /* Configure the client based on our command line. */
	 var config *rest.Config
	 var err error
	 if *master != "" || *kubeconfig != "" {
		 glog.Infof("using out-of-cluster configuration")
		 config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	 } else {
		 glog.Infof("using in-cluster configuration; use -master or -kubeconfig to change")
		 config, err = rest.InClusterConfig()
	 }
	 if err != nil {
		 glog.Fatalf("failed to create config: %v", err)
	 }
	 clientset, err := kubernetes.NewForConfig(config)
	 if err != nil {
		 glog.Fatalf("failed to create client: %v", err)
	 }
 
	 /*
	  * The controller needs to know what the server version is because out-of-tree
	  * provisioners aren't officially supported until 1.5
	  */
	 serverVersion, err := clientset.Discovery().ServerVersion()
	 if err != nil {
		 glog.Fatalf("error getting server version: %v", err)
	 }
 
	 /*
	  * Default provisioner id to the name; the user can override with the
	  * -id option.
	  */
	 prID := provisionerName
	 if *id != "" {
		 prID = *id
	 }
 
	 prName := provisionerName
	 if *name != "" {
		 prName = *name
	 }
 
	 /*
	  * Create the provisioner, which has a standard interface (Provision,
	  * Delete) used by the controller to notify us what to do.
	  */
	 hostPathProvisioner := NewHostPathProvisioner(clientset, prID)
 
	 /* Start the controller */
	 pc := controller.NewProvisionController(
		 clientset,
		 prName,
		 hostPathProvisioner,
		 serverVersion.GitVersion)
 
	 pc.Run(wait.NeverStop)
 }
 