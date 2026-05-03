#!/bin/bash
# TodoPro Build Script for Linux/Mac

echo ""
echo "========================================"
echo "TodoPro - Full Stack Task Management"
echo "========================================"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "[ERROR] Go is not installed"
    echo "Please install Go from https://golang.org/dl"
    exit 1
fi

echo "[1] Building backend..."
go build -o todopro main.go
if [ $? -ne 0 ]; then
    echo "[ERROR] Build failed!"
    exit 1
fi

echo "[OK] Backend built successfully: todopro"
echo ""
echo "[2] To run the application:"
echo ""
echo "    Terminal 1 (Backend):"
echo "    ./todopro"
echo ""
echo "    Terminal 2 (Frontend):"
echo "    python3 -m http.server 8000"
echo ""
echo "Then open http://localhost:8000 in your browser"
echo ""
echo "[3] API available at: http://localhost:5000"
echo ""
