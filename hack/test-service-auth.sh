#!/usr/bin/env bash

ENDPOINT="https://httpbin.syscd.live/"
CF_CLIENT_ID="${CF_CLIENT_ID:-""}"
CF_CLIENT_SECRET="${CF_CLIENT_SECRET:-""}"

curl "$ENDPOINT" \
    -H "CF-Access-Client-Id: $CLIENT_ID" \
    -H "CF-Access-Client-Secret: $CLIENT_SECRET" \
    -H "User-Agent: Mozilla/5.0" \
    -v