#!/bin/bash

echo "Annotating resources for force deletion..."
kubectl -n rook-ceph annotate cephfilesystem ceph-filesystem rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephblockpool ceph-blockpool rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephfilesystemsubvolumegroup ceph-filesystem-csi rook.io/force-deletion="true"

echo "Attempting force deletion..."
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore &
kubectl -n rook-ceph delete cephfilesystem ceph-filesystem &
kubectl -n rook-ceph delete cephblockpool ceph-blockpool &
kubectl -n rook-ceph delete cephcluster rook-ceph &

echo "Removing finalizers..."
# Remove finalizers from dependent resources first
kubectl get cephblockpools.ceph.rook.io ceph-blockpool -o json | jq '.metadata.finalizers = null' | kubectl apply -f -
kubectl get cephfilesystems.ceph.rook.io ceph-filesystem -o json | jq '.metadata.finalizers = null' | kubectl apply -f -
kubectl get cephobjectstores.ceph.rook.io ceph-objectstore -o json | jq '.metadata.finalizers = null' | kubectl apply -f -
kubectl get cephcluster.ceph.rook.io rook-ceph -o json | jq '.metadata.finalizers = null' | kubectl apply -f -


echo """
# Remove rook directory
sudo rm -rvf /var/lib/rook

# Remove logical volumes
sudo lvs --noheadings -o lv_path | grep ceph- | xargs -r -I {} sudo lvremove -f {}

# Remove volume groups
sudo vgs --noheadings -o vg_name | grep ceph- | xargs -r -I {} sudo vgremove -f {}

# Remove physical volume labels
sudo pvs --noheadings -o pv_name | xargs -r -I {} sudo pvremove -f {} 
"""

# echo "Last resort: "
# kubectl proxy --port=8080
# curl -k -X DELETE http://127.0.0.1:8080/apis/ceph.rook.io/v1/namespaces/rook-ceph/cephblockpools/builtin-mgr