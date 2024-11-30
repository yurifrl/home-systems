#!/usr/bin/env bash

# 1. Delete CephBlockPools first
kubectl get cephblockpool -n rook-ceph -o name | xargs -I {} kubectl patch {} -n rook-ceph --type json -p '[{"op":"remove","path":"/metadata/finalizers"}]'
kubectl delete cephblockpool --all -n rook-ceph

# 2. Delete CephCluster
kubectl patch cephcluster my-cluster -n rook-ceph --type json -p '[{"op":"remove","path":"/metadata/finalizers"}]'
kubectl delete cephcluster my-cluster -n rook-ceph

# 3. Delete the namespace if needed
kubectl delete namespace rook-ceph

# 4. If still stuck, force delete everything
kubectl get crd | grep ceph.rook.io | awk '{print $1}' 