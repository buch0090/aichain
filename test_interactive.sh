#!/bin/bash

echo "=== AIChain Interactive Test ==="
echo ""
echo "Choose test method:"
echo "1) Full test with dummy API key (TUI will start)"
echo "2) CLI commands only"
echo "3) Cancel"
echo ""
read -p "Choice (1-3): " choice

case $choice in
    1)
        echo "Starting AIChain with dummy API key..."
        echo "Press 'q' to quit when it starts"
        echo ""
        read -p "Press Enter to continue..."
        CLAUDE_API_KEY=test ./aichain-standalone
        ;;
    2)
        echo "Testing CLI commands..."
        echo ""
        echo "=== Help ==="
        ./aichain-standalone --help
        echo ""
        echo "=== Version ==="
        ./aichain-standalone version
        echo ""
        echo "=== Config ==="
        ./aichain-standalone config --help
        ;;
    3)
        echo "Cancelled."
        exit 0
        ;;
    *)
        echo "Invalid choice"
        exit 1
        ;;
esac