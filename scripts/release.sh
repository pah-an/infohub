#!/bin/bash

# InfoHub Release Script
# Usage: ./scripts/release.sh [patch|minor|major]

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CURRENT_VERSION=""
NEW_VERSION=""
RELEASE_TYPE="${1:-patch}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Check if git repo is clean
check_git_clean() {
    if [ -n "$(git status --porcelain)" ]; then
        log_error "Git working directory is not clean. Please commit or stash changes."
        exit 1
    fi
    
    if [ "$(git rev-parse --abbrev-ref HEAD)" != "main" ]; then
        log_warn "You are not on the main branch. Current branch: $(git rev-parse --abbrev-ref HEAD)"
        read -p "Continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

# Get current version from git tags
get_current_version() {
    local version
    version=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
    CURRENT_VERSION="${version#v}"  # Remove 'v' prefix
    log_info "Current version: $CURRENT_VERSION"
}

# Calculate next version
calculate_next_version() {
    local major minor patch
    IFS='.' read -r major minor patch <<< "$CURRENT_VERSION"
    
    case "$RELEASE_TYPE" in
        major)
            NEW_VERSION="$((major + 1)).0.0"
            ;;
        minor)
            NEW_VERSION="$major.$((minor + 1)).0"
            ;;
        patch)
            NEW_VERSION="$major.$minor.$((patch + 1))"
            ;;
        *)
            log_error "Invalid release type: $RELEASE_TYPE. Use: patch, minor, or major"
            exit 1
            ;;
    esac
    
    log_info "New version will be: $NEW_VERSION"
}

# Run tests
run_tests() {
    log_step "Running tests..."
    cd "$PROJECT_DIR"
    
    # Run linting
    if command -v golangci-lint &> /dev/null; then
        log_info "Running linter..."
        golangci-lint run --timeout=5m
    else
        log_warn "golangci-lint not found, skipping linting"
    fi
    
    # Run tests
    log_info "Running unit tests..."
    go test -v -race ./...
    
    # Run security checks
    if command -v govulncheck &> /dev/null; then
        log_info "Running vulnerability check..."
        govulncheck ./...
    else
        log_warn "govulncheck not found, skipping security check"
    fi
    
    log_info "All tests passed!"
}

# Update version in files
update_version_files() {
    log_step "Updating version in files..."
    
    # Update version in main.go if it exists
    if [ -f "$PROJECT_DIR/cmd/infohub/main.go" ]; then
        sed -i.bak "s/version.*=.*\".*\"/version = \"$NEW_VERSION\"/" "$PROJECT_DIR/cmd/infohub/main.go"
        rm -f "$PROJECT_DIR/cmd/infohub/main.go.bak"
        log_info "Updated version in cmd/infohub/main.go"
    fi
    
    # Update version in Helm chart if it exists
    if [ -f "$PROJECT_DIR/helm/infohub/Chart.yaml" ]; then
        sed -i.bak "s/version:.*/version: $NEW_VERSION/" "$PROJECT_DIR/helm/infohub/Chart.yaml"
        sed -i.bak "s/appVersion:.*/appVersion: $NEW_VERSION/" "$PROJECT_DIR/helm/infohub/Chart.yaml"
        rm -f "$PROJECT_DIR/helm/infohub/Chart.yaml.bak"
        log_info "Updated version in Helm chart"
    fi
    
    # Update package.json if it exists (for any JS tooling)
    if [ -f "$PROJECT_DIR/package.json" ]; then
        sed -i.bak "s/\"version\": \".*\"/\"version\": \"$NEW_VERSION\"/" "$PROJECT_DIR/package.json"
        rm -f "$PROJECT_DIR/package.json.bak"
        log_info "Updated version in package.json"
    fi
}

# Generate changelog entry
update_changelog() {
    log_step "Updating CHANGELOG.md..."
    
    local changelog_file="$PROJECT_DIR/CHANGELOG.md"
    local temp_file="/tmp/changelog_temp.md"
    local release_date
    release_date=$(date +%Y-%m-%d)
    
    if [ ! -f "$changelog_file" ]; then
        log_warn "CHANGELOG.md not found, creating new one"
        cat > "$changelog_file" << EOF
# Changelog

All notable changes to InfoHub will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [$NEW_VERSION] - $release_date

### Added
- Initial release

EOF
        return
    fi
    
    # Create new changelog entry
    {
        head -n 7 "$changelog_file"  # Keep header
        echo ""
        echo "## [$NEW_VERSION] - $release_date"
        echo ""
        echo "### Added"
        echo "- [Add your changes here]"
        echo ""
        echo "### Changed"
        echo "- [Add your changes here]"
        echo ""
        echo "### Fixed"
        echo "- [Add your changes here]"
        echo ""
        tail -n +8 "$changelog_file"  # Rest of file
    } > "$temp_file"
    
    mv "$temp_file" "$changelog_file"
    log_info "Added new release entry to CHANGELOG.md"
    
    # Open editor for user to edit changelog
    if [ -n "$EDITOR" ]; then
        log_info "Opening CHANGELOG.md in $EDITOR for editing..."
        $EDITOR "$changelog_file"
    else
        log_warn "EDITOR not set. Please manually edit CHANGELOG.md before continuing."
        read -p "Press Enter when you've finished editing CHANGELOG.md..."
    fi
}

# Build and test
build_project() {
    log_step "Building project..."
    cd "$PROJECT_DIR"
    
    # Generate swagger docs if swag is available
    if command -v swag &> /dev/null; then
        log_info "Generating Swagger documentation..."
        swag init -g cmd/infohub/main.go -o docs --parseInternal
    fi
    
    # Build binary
    log_info "Building binary..."
    make build
    
    # Build Docker image
    log_info "Building Docker image..."
    docker build -t "infohub:$NEW_VERSION" \
        --build-arg VERSION="$NEW_VERSION" \
        --build-arg COMMIT="$(git rev-parse HEAD)" \
        --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
    
    log_info "Build completed successfully!"
}

# Create git tag and commit
create_release() {
    log_step "Creating release..."
    
    # Add changed files
    git add -A
    
    # Commit changes
    git commit -m "chore: release v$NEW_VERSION

- Update version to $NEW_VERSION
- Update CHANGELOG.md
- Update version files"
    
    # Create annotated tag
    git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION

See CHANGELOG.md for details."
    
    log_info "Created git tag v$NEW_VERSION"
}

# Push to remote
push_release() {
    log_step "Pushing release..."
    
    # Push commits and tags
    git push origin "$(git rev-parse --abbrev-ref HEAD)"
    git push origin "v$NEW_VERSION"
    
    log_info "Pushed release to remote repository"
}

# Create GitHub release (if gh CLI is available)
create_github_release() {
    if command -v gh &> /dev/null; then
        log_step "Creating GitHub release..."
        
        # Extract changelog for this version
        local release_notes
        release_notes=$(awk "/## \[$NEW_VERSION\]/,/## \[/{if(/## \[/ && !/## \[$NEW_VERSION\]/) exit; if(!/## \[$NEW_VERSION\]/) print}" "$PROJECT_DIR/CHANGELOG.md")
        
        gh release create "v$NEW_VERSION" \
            --title "Release v$NEW_VERSION" \
            --notes "$release_notes" \
            --draft=false \
            --prerelease=false
        
        log_info "Created GitHub release"
    else
        log_warn "GitHub CLI not found. Please create release manually at:"
        log_warn "https://github.com/your-org/infohub/releases/new?tag=v$NEW_VERSION"
    fi
}

# Main release process
main() {
    echo "🚀 InfoHub Release Script"
    echo "========================="
    echo "Release type: $RELEASE_TYPE"
    echo ""
    
    # Preflight checks
    log_step "Running preflight checks..."
    check_git_clean
    get_current_version
    calculate_next_version
    
    # Confirmation
    echo ""
    log_warn "This will create a new $RELEASE_TYPE release:"
    log_warn "  Current version: $CURRENT_VERSION"
    log_warn "  New version:     $NEW_VERSION"
    echo ""
    read -p "Continue? (y/N): " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Release cancelled"
        exit 0
    fi
    
    # Release process
    run_tests
    update_version_files
    update_changelog
    build_project
    create_release
    
    # Push release
    echo ""
    read -p "Push release to remote repository? (y/N): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        push_release
        create_github_release
    else
        log_info "Release created locally. Push manually when ready:"
        log_info "  git push origin $(git rev-parse --abbrev-ref HEAD)"
        log_info "  git push origin v$NEW_VERSION"
    fi
    
    echo ""
    log_info "🎉 Release v$NEW_VERSION completed successfully!"
    echo ""
    echo "Next steps:"
    echo "  1. Monitor CI/CD pipeline"
    echo "  2. Verify deployment to staging"
    echo "  3. Deploy to production"
    echo "  4. Update documentation"
    echo "  5. Announce release"
}

# Show help
show_help() {
    cat << EOF
InfoHub Release Script

Usage: $0 [TYPE]

ARGUMENTS:
    TYPE    Release type: patch, minor, or major (default: patch)

EXAMPLES:
    $0 patch    # 1.0.0 -> 1.0.1
    $0 minor    # 1.0.0 -> 1.1.0  
    $0 major    # 1.0.0 -> 2.0.0

REQUIREMENTS:
    - Clean git working directory
    - On main branch (recommended)
    - All tests passing
    - golangci-lint (optional)
    - govulncheck (optional)
    - gh CLI (optional, for GitHub releases)

EOF
}

# Parse arguments
case "${1:-}" in
    -h|--help)
        show_help
        exit 0
        ;;
    patch|minor|major)
        RELEASE_TYPE="$1"
        ;;
    "")
        RELEASE_TYPE="patch"
        ;;
    *)
        log_error "Invalid release type: $1"
        show_help
        exit 1
        ;;
esac

# Check dependencies
if ! command -v git &> /dev/null; then
    log_error "git is required but not installed"
    exit 1
fi

if ! command -v go &> /dev/null; then
    log_error "go is required but not installed"
    exit 1
fi

if ! command -v docker &> /dev/null; then
    log_error "docker is required but not installed"
    exit 1
fi

# Run main function
cd "$PROJECT_DIR"
main "$@"
