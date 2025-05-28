#!/bin/bash

# InfoHub Health Check Script
# Usage: ./scripts/health-check.sh [URL]

set -e

# Configuration
DEFAULT_URL="http://localhost:8080"
URL="${1:-$DEFAULT_URL}"
TIMEOUT=10
MAX_RETRIES=3

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

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

check_endpoint() {
    local endpoint="$1"
    local expected_status="${2:-200}"
    local description="$3"
    
    echo -n "Checking $description... "
    
    local response
    local http_code
    
    response=$(curl -s -w "%{http_code}" --max-time $TIMEOUT "$URL$endpoint" 2>/dev/null || echo "000")
    http_code="${response: -3}"
    
    if [ "$http_code" = "$expected_status" ]; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${RED}✗ (HTTP $http_code)${NC}"
        return 1
    fi
}

check_json_response() {
    local endpoint="$1"
    local expected_field="$2"
    local description="$3"
    
    echo -n "Checking $description... "
    
    local response
    response=$(curl -s --max-time $TIMEOUT "$URL$endpoint" 2>/dev/null || echo "{}")
    
    if echo "$response" | jq -e ".$expected_field" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${RED}✗ (Missing field: $expected_field)${NC}"
        return 1
    fi
}

wait_for_service() {
    local retries=0
    
    log_info "Waiting for service to be ready..."
    
    while [ $retries -lt $MAX_RETRIES ]; do
        if curl -s --max-time $TIMEOUT "$URL/health/live" > /dev/null 2>&1; then
            log_info "Service is responding"
            return 0
        fi
        
        retries=$((retries + 1))
        log_warn "Attempt $retries/$MAX_RETRIES failed, retrying in 5 seconds..."
        sleep 5
    done
    
    log_error "Service failed to respond after $MAX_RETRIES attempts"
    return 1
}

main() {
    echo "🏥 InfoHub Health Check"
    echo "======================="
    echo "Target URL: $URL"
    echo "Timeout: ${TIMEOUT}s"
    echo ""
    
    local failed_checks=0
    
    # Wait for service to be ready
    if ! wait_for_service; then
        log_error "Service is not responding, aborting health check"
        exit 1
    fi
    
    echo ""
    log_info "Running health checks..."
    echo ""
    
    # Basic health checks
    check_endpoint "/health/live" "200" "Liveness probe" || ((failed_checks++))
    check_endpoint "/health/ready" "200" "Readiness probe" || ((failed_checks++))
    check_endpoint "/api/v1/healthz" "200" "API health check" || ((failed_checks++))
    
    # API functionality checks
    check_json_response "/api" "service" "API info endpoint" || ((failed_checks++))
    check_json_response "/api/v1/healthz" "status" "Health check response" || ((failed_checks++))
    
    # Optional authenticated endpoint check (if auth is disabled)
    if curl -s --max-time $TIMEOUT "$URL/api/v1/news" | jq -e ".version" > /dev/null 2>&1; then
        check_json_response "/api/v1/news" "version" "News endpoint" || ((failed_checks++))
    else
        echo -n "Checking news endpoint (with auth)... "
        echo -e "${YELLOW}SKIPPED (Auth required)${NC}"
    fi
    
    # Metrics endpoint
    check_endpoint "/metrics" "200" "Metrics endpoint" || ((failed_checks++))
    
    # Swagger documentation
    check_endpoint "/swagger/" "200" "Swagger documentation" || ((failed_checks++))
    
    echo ""
    
    # Summary
    if [ $failed_checks -eq 0 ]; then
        log_info "All health checks passed! ✨"
        echo ""
        echo "Service is healthy and ready to serve traffic."
        exit 0
    else
        log_error "$failed_checks health check(s) failed!"
        echo ""
        echo "Service may not be fully operational."
        exit 1
    fi
}

# Help function
show_help() {
    cat << EOF
InfoHub Health Check Script

Usage: $0 [OPTIONS] [URL]

OPTIONS:
    -h, --help     Show this help message
    -t, --timeout  Set timeout in seconds (default: $TIMEOUT)
    -r, --retries  Set max retries (default: $MAX_RETRIES)

ARGUMENTS:
    URL            Base URL to check (default: $DEFAULT_URL)

EXAMPLES:
    $0                                    # Check localhost
    $0 http://infohub.example.com        # Check production
    $0 -t 30 http://staging.example.com  # Check with custom timeout

EXIT CODES:
    0  All checks passed
    1  One or more checks failed
    2  Service not responding

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -r|--retries)
            MAX_RETRIES="$2"
            shift 2
            ;;
        -*)
            log_error "Unknown option: $1"
            show_help
            exit 2
            ;;
        *)
            URL="$1"
            shift
            ;;
    esac
done

# Check dependencies
if ! command -v curl &> /dev/null; then
    log_error "curl is required but not installed"
    exit 2
fi

if ! command -v jq &> /dev/null; then
    log_error "jq is required but not installed"
    exit 2
fi

# Run main function
main "$@"
