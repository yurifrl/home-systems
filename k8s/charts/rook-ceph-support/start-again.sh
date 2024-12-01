#!/bin/bash

#!/bin/bash

# Annotate resources for force deletion
kubectl -n rook-ceph annotate cephcluster my-cluster rook.io/force-delete-cluster-on-removal=true
kubectl -n rook-ceph annotate cephblockpool builtin-mgr rook.io/force-delete-pool-on-removal=true

# Delete the resources
kubectl -n rook-ceph delete cephcluster my-cluster
kubectl -n rook-ceph delete cephblockpool builtin-mgr


echo "Removing finalizers..."
kubectl -n rook-ceph patch cephcluster my-cluster --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool builtin-mgr --type merge -p '{"metadata":{"finalizers": []}}'


# Check the resources
kubectl -n rook-ceph get cephcluster,cephblockpool