#!/usr/bin/env bash

# Define your network range here
network="192.168.1.0/24"

# Convert CIDR notation to usable range
subnet=$(echo $network | cut -d '/' -f 1)
range=$(echo $((${subnet##*.}+1))-$((($(echo $network | cut -d '/' -f 2)-1))))

for ip in $(seq $range); do
   host="$subnet.$ip"
   echo "Trying $host..."
   ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no -o BatchMode=yes nixos@$host exit &>/dev/null
   if [ $? -eq 0 ]; then
       echo "$host is reachable."
   else
       echo "$host is not reachable."
   fi
done
