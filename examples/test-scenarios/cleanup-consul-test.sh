#!/bin/bash

# Cleanup script for Consul test environment

echo "=== Cleaning up Consul test environment ==="
echo ""

# Stop backend processes
if [ -f /tmp/consul-test-backends/pids.txt ]; then
    echo "Stopping backend services..."
    while read PID; do
        if kill -0 $PID 2>/dev/null; then
            kill $PID
            echo "✓ Stopped backend process $PID"
        fi
    done < /tmp/consul-test-backends/pids.txt
else
    echo "Stopping all backend processes..."
    pkill -f 'backend-900[1-4].py' && echo "✓ Stopped backend processes"
fi

# Stop Consul container
echo ""
echo "Stopping Consul server..."
docker rm -f consul-test 2>/dev/null && echo "✓ Stopped Consul container"

# Clean up temp files
echo ""
echo "Removing temp files..."
rm -rf /tmp/consul-test-backends
echo "✓ Cleaned up temp files"

echo ""
echo "=== Cleanup complete! ==="
