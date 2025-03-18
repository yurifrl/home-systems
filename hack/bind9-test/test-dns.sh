#!/bin/bash

echo "Testing DNS resolution for syscd.dev domain..."
echo ""

echo "Querying NS record:"
dig @127.0.0.1 syscd.dev NS

echo ""
echo "Querying test.syscd.dev:"
dig @127.0.0.1 test.syscd.dev

echo ""
echo "Querying www.syscd.dev:"
dig @127.0.0.1 www.syscd.dev

echo ""
echo "Testing external domain resolution (recursive query):"
dig @127.0.0.1 google.com 



nsupdate -v -k config/keys.conf update.txt