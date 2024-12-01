#!/bin/bash

echo "Attempting normal deletion..."
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem-csi
kubectl -n rook-ceph delete CephFilesystemSubVolumeGroup ceph-filesystem-csi
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore
kubectl -n rook-ceph delete cephblockpool ceph-blockpool
sleep 30

echo "Attempting force deletion..."
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem --grace-period=0 --force
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem-csi --grace-period=0 --force
kubectl -n rook-ceph delete CephFilesystemSubVolumeGroup ceph-filesystem-csi --grace-period=0 --force
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore --grace-period=0 --force
kubectl -n rook-ceph delete cephblockpool ceph-blockpool --grace-period=0 --force
sleep 30

echo "Removing finalizers..."
kubectl -n rook-ceph patch CephFilesystem ceph-filesystem --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystem ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystemSubVolumeGroup ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephobjectstore ceph-objectstore --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool ceph-blockpool --type merge -p '{"metadata":{"finalizers": []}}'