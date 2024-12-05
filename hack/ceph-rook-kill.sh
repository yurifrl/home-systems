#!/bin/bash

echo "Annotating resources for force deletion..."
kubectl -n rook-ceph annotate cephfilesystem ceph-filesystem rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephblockpool ceph-blockpool rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephfilesystemsubvolumegroup ceph-filesystem-csi rook.io/force-deletion="true"

echo "Attempting force deletion..."
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem --grace-period=0
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem-csi --grace-period=0
kubectl -n rook-ceph delete CephFilesystemSubVolumeGroup ceph-filesystem-csi --grace-period=0
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore --grace-period=0
kubectl -n rook-ceph delete cephblockpool ceph-blockpool --grace-period=0

echo "Removing finalizers..."
# Remove finalizers from dependent resources first
kubectl -n rook-ceph patch CephBlockPool ceph-blockpool --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystem ceph-filesystem --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystemSubVolumeGroup ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephObjectStore ceph-objectstore --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephCluster rook-ceph --type merge -p '{"metadata":{"finalizers": []}}'


# echo "Last resort: "
# kubectl proxy --port=8080
# curl -k -X DELETE http://127.0.0.1:8080/apis/ceph.rook.io/v1/namespaces/rook-ceph/cephblockpools/builtin-mgr