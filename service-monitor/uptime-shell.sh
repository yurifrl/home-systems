#!/usr/bin/env bash

# Add interval configuration at the top
INTERVAL_SECONDS=5  # Change this value to set your desired interval

# Array of services to check
# services=(
#     "https://grafana.syscd.live"
#     "https://grafana.syscd.tech"
#     "http://prometheus.syscd.live"
#     "http://prometheus.syscd.tech"
#     "http://alertmanager.syscd.live"
#     "http://alertmanager.syscd.tech"
#     "http://kiali.syscd.live"
#     "http://kiali.syscd.tech"
#     "https://pyrra.syscd.live"
#     "https://pyrra.syscd.tech"
#     "https://ceph.syscd.live"
#     "https://ceph.syscd.tech"
#     "https://argocd.syscd.live"
#     "https://argocd.syscd.tech"
#     "https://k8s.syscd.live"
#     "https://k8s.syscd.tech"
#     "nixos-1:9876"
# )

services=(
    "https://argocd.syscd.live"
    "https://argocd.syscd.tech"
    "nixos-1:9876"
)


# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to check URL
check_url() {
    local url=$1
    local response
    local start_time
    local end_time
    local duration
    
    echo -n "Checking $url... "
    
    start_time=$(date +%s.%N)
    response=$(curl -L -s -o /dev/null -w "%{http_code}" --max-time 10 "$url" 2>/dev/null)
    end_time=$(date +%s.%N)
    
    # Calculate duration in milliseconds
    duration=$(echo "($end_time - $start_time) * 1000" | bc | cut -d'.' -f1)
    
    if [ "$response" -ge 200 ] && [ "$response" -lt 300 ]; then
        echo -e "${GREEN}OK${NC} (HTTP $response, ${duration}ms)"
        return 0
    else
        echo -e "${RED}FAILED${NC} (HTTP $response, ${duration}ms)"
        return 1
    fi
}

# Display the version from file

# Main execution
while true; do
    clear  # Clear screen before each run
    
    curl "https://argocd.syscd.live"
    echo "Running health check (every ${INTERVAL_SECONDS} seconds)"
    echo "Script version: $(cat /version.txt)"
    echo "Current time: $(date)"
    echo "-----------------------------------"
    
    failed_services=()
    successful_services=()

    # Sort services alphabetically
    IFS=$'\n' sorted_services=($(sort <<<"${services[*]}"))
    unset IFS

    for url in "${sorted_services[@]}"; do
        if ! check_url "$url" "$url"; then
            failed_services+=("$url")
        else
            successful_services+=("$url")
        fi
    done

    echo "-----------------------------------"
    echo "Health check complete!"

    # Print summary
    echo
    echo "SUMMARY:"
    echo "--------"
    echo -e "${GREEN}Successful (${#successful_services[@]})${NC}"
    for url in "${successful_services[@]}"; do
        echo "✓ $url"
    done

    if [ ${#failed_services[@]} -gt 0 ]; then
        echo
        echo -e "${RED}Failed (${#failed_services[@]})${NC}"
        for url in "${failed_services[@]}"; do
            echo "✗ $url"
        done
    fi

    echo
    echo "Next check in ${INTERVAL_SECONDS} seconds..."
    sleep ${INTERVAL_SECONDS}s
done

exit 0