#!/bin/bash

echo "Attempting normal deletion..."
kubectl -n rook-ceph delete cephcluster my-cluster
kubectl -n rook-ceph delete cephblockpool builtin-mgr

echo "Attempting force deletion..."
kubectl -n rook-ceph delete cephcluster my-cluster --grace-period=0 --force
kubectl -n rook-ceph delete cephblockpool builtin-mgr --grace-period=0 --force

echo "Removing finalizers..."
kubectl -n rook-ceph patch cephcluster my-cluster --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool builtin-mgr --type merge -p '{"metadata":{"finalizers": []}}'

# echo "Last resort: "
# kubectl proxy --port=8080 &
# PROXY_PID=$!
# curl -k -X DELETE http://127.0.0.1:8080/apis/ceph.rook.io/v1/namespaces/rook-ceph/cephclusters/my-cluster
# curl -k -X DELETE http://127.0.0.1:8080/apis/ceph.rook.io/v1/namespaces/rook-ceph/cephblockpools/builtin-mgr
# kill $PROXY_PID



kubectl -n rook-ceph get cephcluster,cephblockpool
