#!/bin/bash

echo "=== Mantle Withdrawal Scheduler ==="
echo ""

# Load environment variables from .env file
if [ -f .env ]; then
	export $(grep -v '^#' .env | xargs)
fi

# Build the scheduler
echo "📦 Building scheduler..."
go build -o mantle-scheduler scheduler.go

if [ $? -ne 0 ]; then
	echo "❌ Build failed"
	exit 1
fi

echo "✅ Build successful"
echo ""

# Check command
if [ "$1" == "" ]; then
	echo "Usage:"
	echo "  ./run-scheduler.sh check             - Run a single check"
	echo "  ./run-scheduler.sh start             - Start the scheduler (runs continuously)"
	echo ""
	echo "Environment Variables:"
	echo "  WITHDRAWAL_TX_HASH - Withdrawal transaction hash(es) to monitor (comma-separated for multiple)"
	echo ""
	echo "Examples:"
	echo "  # Single withdrawal"
	echo "  export WITHDRAWAL_TX_HASH=0xe0c400563d9a70f84966622f13a5560bfacfe9621ea554ee7939fd06d2e10417"
	echo "  # Multiple withdrawals"
	echo "  export WITHDRAWAL_TX_HASH=0xe0c400563d9a70f84966622f13a5560bfacfe9621ea554ee7939fd06d2e10417,0xabc123..."
	echo "  ./run-scheduler.sh check"
	echo "  ./run-scheduler.sh start"
	exit 0
fi

# Run the scheduler
echo "🚀 Running scheduler..."
echo ""
./mantle-scheduler "$@"
