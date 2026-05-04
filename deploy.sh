#!/bin/bash

# TodoPro Deployment Script
# Usage: ./deploy.sh [command]
# Commands: setup, build, run, stop, clean, test

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="docker-compose.yml"
PROJECT_NAME="todopro"

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "docker-compose is not installed. Please install docker-compose."
        exit 1
    fi
}

check_env() {
    if [ ! -f .env ]; then
        log_warn ".env file not found. Creating from .env.example..."
        if [ -f .env.example ]; then
            cp .env.example .env
            log_info ".env created. Please edit it with your configuration."
        else
            log_error ".env.example not found. Cannot create .env"
            exit 1
        fi
    fi
}

# Commands
cmd_setup() {
    log_info "Setting up TodoPro..."
    
    check_docker
    check_env
    
    # Start services
    log_info "Starting PostgreSQL and backend..."
    docker-compose -f "$COMPOSE_FILE" up -d postgres
    
    # Wait for PostgreSQL to be ready
    log_info "Waiting for database to be ready..."
    sleep 5
    
    # Check if migrations ran
    log_info "Database is ready. You can now run:"
    echo ""
    echo "  ./deploy.sh run         # Start the app"
    echo "  ./deploy.sh logs        # View logs"
    echo "  ./deploy.sh stop        # Stop everything"
    echo "  ./deploy.sh clean       # Remove all data"
    echo ""
}

cmd_build() {
    log_info "Building Docker images..."
    docker-compose -f "$COMPOSE_FILE" build
    log_info "Build complete."
}

cmd_run() {
    log_info "Starting TodoPro services..."
    check_env
    docker-compose -f "$COMPOSE_FILE" up -d
    log_info "Services started."
    log_info "App: http://localhost:5000"
    log_info "API: http://localhost:5000/api"
    log_info "Health: http://localhost:5000/health"
    log_info ""
    log_info "To view logs: ./deploy.sh logs"
}

cmd_stop() {
    log_info "Stopping services..."
    docker-compose -f "$COMPOSE_FILE" down
    log_info "Services stopped."
}

cmd_logs() {
    docker-compose -f "$COMPOSE_FILE" logs -f
}

cmd_clean() {
    log_warn "This will delete all data. Are you sure? (yes/no)"
    read -r confirm
    if [ "$confirm" = "yes" ]; then
        log_info "Removing containers and volumes..."
        docker-compose -f "$COMPOSE_FILE" down -v
        log_info "Clean complete."
    else
        log_info "Cancelled."
    fi
}

cmd_test() {
    log_info "Running tests..."
    
    # Check if backend is running
    if ! curl -s http://localhost:5000/health > /dev/null; then
        log_error "Backend is not running. Start it first with: ./deploy.sh run"
        exit 1
    fi
    
    # Test health endpoint
    log_info "Testing health endpoint..."
    if curl -f http://localhost:5000/health > /dev/null; then
        log_info "✓ Health check passed"
    else
        log_error "✗ Health check failed"
        exit 1
    fi
    
    # Test database connection
    log_info "Testing database connection..."
    docker-compose -f "$COMPOSE_FILE" exec -T postgres psql -U todopro -d todopro -c "SELECT 1;" > /dev/null 2>&1
    if [ $? -eq 0 ]; then
        log_info "✓ Database connection OK"
    else
        log_error "✗ Database connection failed"
        exit 1
    fi
    
    # Test registration flow
    log_info "Testing API endpoints..."
    
    # Register a test user
    RESPONSE=$(curl -s -X POST http://localhost:5000/api/register \
        -H "Content-Type: application/json" \
        -d '{"email":"test@test.com","password":"Test12345","first_name":"Test","last_name":"User"}')
    
    if echo "$RESPONSE" | grep -q "token"; then
        log_info "✓ Registration works"
    else
        log_error "✗ Registration failed: $RESPONSE"
        exit 1
    fi
    
    # Extract token and test profile
    TOKEN=$(echo "$RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    PROFILE=$(curl -s http://localhost:5000/api/profile \
        -H "Authorization: Bearer $TOKEN")
    
    if echo "$PROFILE" | grep -q "test@test.com"; then
        log_info "✓ Authentication works"
    else
        log_error "✗ Authentication failed"
        exit 1
    fi
    
    log_info ""
    log_info "All tests passed! ✓"
}

cmd_shell() {
    log_info "Opening shell in backend container..."
    docker-compose -f "$COMPOSE_FILE" exec backend sh
}

cmd_db_shell() {
    log_info "Opening psql in database container..."
    docker-compose -f "$COMPOSE_FILE" exec postgres psql -U todopro -d todopro
}

cmd_backup() {
    BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).sql"
    log_info "Creating backup to $BACKUP_FILE..."
    docker-compose -f "$COMPOSE_FILE" exec -T postgres pg_dump -U todopro todopro > "$BACKUP_FILE"
    log_info "Backup saved to $BACKUP_FILE"
}

# Main
case "${1:-help}" in
    setup)
        cmd_setup
        ;;
    build)
        cmd_build
        ;;
    run)
        cmd_run
        ;;
    stop)
        cmd_stop
        ;;
    logs)
        cmd_logs
        ;;
    clean)
        cmd_clean
        ;;
    test)
        cmd_test
        ;;
    shell)
        cmd_shell
        ;;
    db)
        cmd_db_shell
        ;;
    backup)
        cmd_backup
        ;;
    *)
        echo "TodoPro Deployment Script"
        echo ""
        echo "Usage: ./deploy.sh [command]"
        echo ""
        echo "Commands:"
        echo "  setup    - Initial setup (creates .env if needed)"
        echo "  build    - Build Docker images"
        echo "  run      - Start all services"
        echo "  stop     - Stop all services"
        echo "  logs     - Show service logs"
        echo "  test     - Run health checks"
        echo "  shell    - Open shell in backend container"
        echo "  db       - Open psql shell in database"
        echo "  backup   - Create database backup"
        echo "  clean    - Remove all data (dangerous!)"
        echo "  help     - Show this help"
        echo ""
        echo "Examples:"
        echo "  ./deploy.sh setup     # First time setup"
        echo "  ./deploy.sh run       # Start app"
        echo "  ./deploy.sh test      # Verify running"
        echo ""
        exit 1
        ;;
esac
