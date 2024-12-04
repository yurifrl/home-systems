#!/bin/bash

# Annotate resources for force deletion
kubectl -n rook-ceph annotate cephcluster my-cluster rook.io/force-delete-cluster-on-removal=true
kubectl -n rook-ceph annotate cephblockpool builtin-mgr rook.io/force-delete-pool-on-removal=true
kubectl -n rook-ceph annotate cephblockpool ceph-blockpool rook.io/force-delete-pool-on-removal=true
kubectl -n rook-ceph annotate cephfilesystem ceph-filesystem rook.io/force-delete-filesystem-on-removal=true
kubectl -n rook-ceph annotate cephfilesystemsubvolumegroup ceph-filesystem-csi rook.io/force-delete-group-on-removal=true
kubectl -n rook-ceph annotate cephobjectstore ceph-objectstore rook.io/force-delete-store-on-removal=true

# Delete the resources
kubectl -n rook-ceph delete cephcluster my-cluster
kubectl -n rook-ceph delete cephblockpool builtin-mgr
kubectl -n rook-ceph delete cephblockpool ceph-blockpool
kubectl -n rook-ceph delete cephfilesystem ceph-filesystem
kubectl -n rook-ceph delete cephfilesystemsubvolumegroup ceph-filesystem-csi
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore

echo "Removing finalizers..."
kubectl -n rook-ceph patch cephcluster my-cluster --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool builtin-mgr --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool ceph-blockpool --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephfilesystem ceph-filesystem --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephfilesystemsubvolumegroup ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephobjectstore ceph-objectstore --type merge -p '{"metadata":{"finalizers": []}}'

# Check the resources
kubectl -n rook-ceph get cephcluster,cephblockpool,cephfilesystem,cephfilesystemsubvolumegroup,cephobjectstore