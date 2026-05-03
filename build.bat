@echo off
REM TodoPro Build Script for Windows

echo.
echo ========================================
echo TodoPro - Full Stack Task Management
echo ========================================
echo.

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl
    pause
    exit /b 1
)

echo [1] Building backend...
go build -o todopro.exe main.go
if errorlevel 1 (
    echo [ERROR] Build failed!
    pause
    exit /b 1
)

echo [OK] Backend built successfully: todopro.exe
echo.
echo [2] To run the application:
echo.
echo    Terminal 1 (Backend):
echo    .\todopro.exe
echo.
echo    Terminal 2 (Frontend):
echo    python -m http.server 8000
echo.
echo Then open http://localhost:8000 in your browser
echo.
echo [3] API available at: http://localhost:5000
echo.
pause
