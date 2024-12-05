#!/usr/bin/env bash

# Prompt for confirmation
read -p "Are you sure you want to force delete Ceph resources? This action cannot be undone. (y/n): " confirm
if [[ $confirm != "y" ]]; then
  echo "Operation cancelled."
  exit 0
fi

echo "Annotating resources for force deletion..."
kubectl -n rook-ceph annotate cephfilesystem ceph-filesystem rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephblockpool ceph-blockpool rook.io/force-deletion="true"
kubectl -n rook-ceph annotate cephfilesystemsubvolumegroup ceph-filesystem-csi rook.io/force-deletion="true"

echo "Attempting force deletion..."
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem --grace-period=0 &
kubectl -n rook-ceph delete CephFilesystem ceph-filesystem-csi --grace-period=0 &
kubectl -n rook-ceph delete CephFilesystemSubVolumeGroup ceph-filesystem-csi --grace-period=0 &
kubectl -n rook-ceph delete cephobjectstore ceph-objectstore --grace-period=0 &
kubectl -n rook-ceph delete cephblockpool ceph-blockpool --grace-period=0 &

echo "Removing finalizers..."
kubectl -n rook-ceph patch CephFilesystem ceph-filesystem --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystem ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch CephFilesystemSubVolumeGroup ceph-filesystem-csi --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephobjectstore ceph-objectstore --type merge -p '{"metadata":{"finalizers": []}}'
kubectl -n rook-ceph patch cephblockpool ceph-blockpool --type merge -p '{"metadata":{"finalizers": []}}'

echo "Checking if resources were deleted..."
kubectl -n rook-ceph get cephobjectstore,cephfilesystem,cephblockpool,cephcluster 

echo """
# Check if resources were deleted
kubectl -n rook-ceph get cephobjectstore,cephfilesystem,cephblockpool,cephcluster 

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