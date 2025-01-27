#!/usr/bin/env bash

ENDPOINT="https://httpbin.syscd.live/status/200"
if [[ -z "${CF_CLIENT_ID}" ]] || [[ -z "${CF_CLIENT_SECRET}" ]]; then
  echo "Error: CF_CLIENT_ID and CF_CLIENT_SECRET environment variables must be set"
  exit 1
fi


curl "$ENDPOINT" \
    -H "CF-Access-Client-Id: $CF_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $CF_CLIENT_SECRET" \
    -H "User-Agent: Mozilla/5.0" \
    -I