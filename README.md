# Kubernetes dynamic hostpath provisioner

This is a Persistent Volume Claim (PVC) provisioner for Kubernetes. It dynamically provisions hostPath volumes to provide storage for PVCs. Please see [Original README](#original-readme) for more information.

## Test environment

- IBM Cloud Private 3.1 (uses Kubernetes 1.11.1) with three worker nodes.
- Each worker node has shared NFS directory.
- [deployment.yaml](deployment/deployment.yaml) used to deploy provisioner to ICP.
- [storageclass.yaml](deployment/storageclass.yaml) used to create StorageClass for dynamic provisioning.

## Usage

- Deploy provisioner:
  - Change volume paths to your environment. Default is: ```/dynfs```.
  - ```kubectl create -f deployment/deployment.yaml```
- Create storage class:
  - Change name other parameters to your own. Default storage class name is ```dynfs```. 
  - Make sure that *pvDir* is the same as volume paths in deployment.yaml.
  - ```kubectl create -f deployment/storageclass.yaml```
- Test:
  - Create PVC: ```kubectl create -f deployment/testclaim.yaml```
  - Verify that directory was created.
  - Delete PVC: ```kubectl delete -f deployment/testclaim.yaml```
  - Verify that directory was deleted (only if claim policy was Delete).

## Something to keep in mind

- Set up NFS or some other network/shared storage prior to deploying this provisioner.
  - Configure the mounted directory, or directory undert the mounted directory, in [deployment.yaml](deployment/deployment.yaml) and [storageclass.yaml](deployment/storageclass.yaml).
- Provisioner uses privileged container.
  - [deployment.yaml](deployment/deployment.yaml) creates PodSecurityPolicy to allow it.
- Paths of dynamically created directories:
  - ```/pvdir/namespace/claim-name```
  - *pvdir*, directory given in StorageClass configuration and it must be mounted in provisioner (see both [deployment.yaml](deployment/deployment.yaml) and [storageclass.yaml](deployment/storageclass.yaml)).
  - *namespace* where PVC will be created. Default is *default* (which is ICP default namespace).
  - *claim-name* is the claim-name given when pod is deployed. If *claim-name* exists, a number is appended.
- Directories that are created have 777 permissions.
- If the mounted directory is deleted while the provisioner is deployed, provisioning may not work.
- Default PersistentVolumeReclaimPolicy is Delete. When claim is deleted so is the provisioned directory.
  - This can be changed in StorageClass configuration.

## Changes from the original

- Changed Dockerfile to build provisioner. No need to install Go on local machine.
- Removed vendor-dir.
- Copied new provisioner code from https://github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/tree/master/examples/hostpath-provisioner and made changes based on the torchbox code. 
- Added sample yaml-files for deployment, storage class and claim.
- PV dir name generated as in https://github.com/nmasse-itix/OpenShift-HostPath-Provisioner/blob/master/src/hostpath-provisioner/hostpath-provisioner.go.
- Logging changes as in  https://www.ardanlabs.com/blog/2013/11/using-log-package-in-go.html.
- And some other changes.

# Original README

Kubernetes hostpath provisioner
===============================

This is a Persistent Volume Claim (PVC) provisioner for Kubernetes.  It
dynamically provisions hostPath volumes to provide storage for PVCs.  It is
based on the
[demo hostpath-provisioner](https://github.com/kubernetes-incubator/external-storage/tree/master/docs/demo/hostpath-provisioner).

Unlike the demo provisioner, this version is intended to be suitable for
production use.  Its purpose is to provision storage on network filesystems
mounted on the host, rather than using Kubernetes' built-in network volume
support.   This has some advantages over storage-specific provisioners:

* There is no need to expose PV credentials to users (cephfs-provisioner
  requires this, for example).

* PVs can be provisioned on network storage not natively supported in
  Kubernetes, e.g. `ceph-fuse`.

* The network storage configuration is centralised on the node (e.g., in
  `/etc/fstab`); this means you can change the storage configuration, or even
  completely change the storage type (e.g. NFS to CephFS) without having to
  update every PV by hand.

There are also some disadvantages:

* Every node requires full access to the storage containing all PVs.  This may
  defeat attempts to limit node access in Kubernetes, such as the Node
  authorizor.

* Storage can no longer be introspected via standard Kubernetes APIs.

* Kubernetes cannot report storage-related errors such as failures to mount
  storage; this information will not be available to users.

* Moving storage configuration from Kubernetes to the host will not work well
  in environments where host access is limited, such as GKE.

We test and use this provisioner with CephFS and `ceph-fuse`, but in principal
it should work with any network filesystem.

You **cannot** use it without a network filesystem unless you can ensure all
provisioned PVs will only be used on the host where they were provisioned; this
is an inherent limitation of `hostPath`.

Unlike the demo hostpath-provisioner, there is no attempt to identify PVs by
node name, because the intended use is with a network filesystem mounted on all
hosts.

Deployment
----------

### Mount the network storage

First, mount the storage on each host.  You can do this any way you like;
systemd mount units or `/etc/fstab` is the typical method.  The storage
**must** be mounted at the same path on every host.

However you decide to provision the storage, you should set the mountpoint
immutable:

```
# mkdir /mynfs
# chattr +i /mynfs
# mount -tnfs mynfsserver:/export/mynfs /mynfs
```

This ensures that nothing can be written to `/mynfs` if the storage is
unmounted.  Without this protection, a failure to mount the storage could
result in PVs being provisioned on the host instead of on the network storage.
This would initially appear to work, but then lead to data loss.

Note that the immutable flag is set on the underlying mountpoint, *not* the
mounted filesystem.  Once the filesystem is mounted, the immutable mountpoint
is hidden and files can be created as normal.

### Create a StorageClass

The provisioner must be associated with a Kubernetes StorageClass:

```
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: cephfs
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: torchbox.com/hostpath
parameters:
  pvDir: /ceph/pvs
```

* The `name` can be anything you like; you could name it after the storage type
  (such as `cephfs-ssd` or `bignfs`), or give it a generic name like
  `pvstorage`.

* If you *don't* want this to be the default storage class, delete the
  `is-default-class` annotation; then this class will only be used if
  explicitly requested in the PVC.

* Set `pvDir` to the root path on the host where volumes should be provisioned.
  This must be on network storage, but does not need to be the root of the
  storage or the mountpoint.

* Unless you're running multiple provisioners, leave `provisioner` at the
  default `torchbox.com/hostpath`.  If you want to run multiple provisioners,
  the value passed to `-name` when starting the provisioner must match the
  value of `provisioner`.

### Start the provisioner

Please see the rest in [the original repository](https://github.com/torchbox/k8s-hostpath-provisioner/tree/fe8dcfde450cfbb505cb7f2044c404bc5b86bbc8).
