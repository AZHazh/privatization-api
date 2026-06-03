#!/bin/bash
# =============================================================================
# Sub2API Docker Deployment Preparation Script
# =============================================================================
# This script prepares deployment files for Sub2API:
#   - Downloads docker-compose.local.yml and .env.example
#   - Generates secure secrets (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD)
#   - Creates necessary data directories
#
# After running this script, you can start services with:
#   docker-compose up -d
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Deployment source and defaults. Override these when deploying a private image or
# when running multiple stacks on one server:
#   SUB2API_IMAGE=registry.example.com/sub2api:20260601 \
#   SUB2API_INSTANCE=sub2api-test \
#   SUB2API_PORT=18080 \
#   bash docker-deploy.sh
GITHUB_RAW_URL="${SUB2API_RAW_URL:-https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy}"
DEFAULT_IMAGE="${SUB2API_IMAGE:-weishaw/sub2api:latest}"
DEFAULT_PORT="${SUB2API_PORT:-${SERVER_PORT:-8080}}"
FORCE="${FORCE:-false}"
FORCE_DOWNLOAD="${SUB2API_FORCE_DOWNLOAD:-false}"

# Print colored message
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Generate random secret
generate_secret() {
    openssl rand -hex 32
}

sanitize_project_name() {
    local raw="${1:-sub2api}"
    local sanitized
    sanitized=$(printf "%s" "$raw" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]/-/g' | sed 's/^[^a-z0-9]*//')
    if [ -z "$sanitized" ]; then
        sanitized="sub2api"
    fi
    printf "%s" "$sanitized"
}

set_env_value() {
    local key="$1"
    local value="$2"
    local escaped
    escaped=$(printf "%s" "$value" | sed 's/[&|]/\\&/g')

    if grep -q "^${key}=" .env; then
        if sed --version >/dev/null 2>&1; then
            sed -i "s|^${key}=.*|${key}=${escaped}|" .env
        else
            sed -i '' "s|^${key}=.*|${key}=${escaped}|" .env
        fi
    else
        printf "\n%s=%s\n" "$key" "$value" >> .env
    fi
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

script_dir() {
    local source="${BASH_SOURCE[0]}"
    while [ -L "$source" ]; do
        local dir
        dir="$(cd -P "$(dirname "$source")" >/dev/null 2>&1 && pwd)"
        source="$(readlink "$source")"
        [[ "$source" != /* ]] && source="$dir/$source"
    done
    cd -P "$(dirname "$source")" >/dev/null 2>&1 && pwd
}

copy_or_download() {
    local file_name="$1"
    local output_name="$2"
    local local_dir="$3"
    local local_path="${local_dir}/${file_name}"

    if [ "${FORCE_DOWNLOAD}" != "true" ] && [ -f "${local_path}" ]; then
        print_info "Using bundled ${file_name}..."
        local output_path
        case "${output_name}" in
            /*) output_path="${output_name}" ;;
            *) output_path="${PWD}/${output_name}" ;;
        esac
        if [ "${local_path}" != "${output_path}" ]; then
            cp "${local_path}" "${output_name}"
        fi
        return
    fi

    print_info "Downloading ${output_name}..."
    if command_exists curl; then
        curl -fsSL "${GITHUB_RAW_URL}/${file_name}" -o "${output_name}"
    elif command_exists wget; then
        wget -q "${GITHUB_RAW_URL}/${file_name}" -O "${output_name}"
    else
        print_error "Neither curl nor wget is installed. Please install one of them."
        exit 1
    fi
}

# Main installation function
main() {
    echo ""
    echo "=========================================="
    echo "  Sub2API Deployment Preparation"
    echo "=========================================="
    echo ""

    # Check if openssl is available
    if ! command_exists openssl; then
        print_error "openssl is not installed. Please install openssl first."
        exit 1
    fi

    local default_instance
    default_instance="$(basename "$PWD")"
    INSTANCE_NAME="$(sanitize_project_name "${SUB2API_INSTANCE:-${COMPOSE_PROJECT_NAME:-$default_instance}}")"

    print_info "Instance name: ${INSTANCE_NAME}"
    print_info "Docker image:   ${DEFAULT_IMAGE}"
    print_info "HTTP port:      ${DEFAULT_PORT}"
    echo ""

    # Check if deployment already exists
    if [ -f "docker-compose.yml" ] && [ -f ".env" ] && [ "${FORCE}" != "true" ]; then
        print_warning "Deployment files already exist in current directory."
        read -p "Overwrite existing files? (y/N): " -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Cancelled."
            exit 0
        fi
    fi

    local bundled_dir
    bundled_dir="$(script_dir)"

    # Download docker-compose.local.yml and save as docker-compose.yml, or use
    # bundled files when this script is distributed with the current branch.
    copy_or_download "docker-compose.local.yml" "docker-compose.yml" "${bundled_dir}"
    print_success "Prepared docker-compose.yml"

    # Download .env.example, or use the bundled copy.
    copy_or_download ".env.example" ".env.example" "${bundled_dir}"
    print_success "Prepared .env.example"

    # Generate .env file with auto-generated secrets
    print_info "Generating secure secrets..."
    echo ""

    # Generate secrets
    JWT_SECRET=$(generate_secret)
    TOTP_ENCRYPTION_KEY=$(generate_secret)
    POSTGRES_PASSWORD=$(generate_secret)

    # Create .env from .env.example
    cp .env.example .env

    set_env_value "COMPOSE_PROJECT_NAME" "${INSTANCE_NAME}"
    set_env_value "SUB2API_IMAGE" "${DEFAULT_IMAGE}"
    set_env_value "SERVER_PORT" "${DEFAULT_PORT}"
    set_env_value "JWT_SECRET" "${JWT_SECRET}"
    set_env_value "TOTP_ENCRYPTION_KEY" "${TOTP_ENCRYPTION_KEY}"
    set_env_value "POSTGRES_PASSWORD" "${POSTGRES_PASSWORD}"

    # Create data directories
    print_info "Creating data directories..."
    mkdir -p data postgres_data redis_data
    print_success "Created data directories"

    # Set secure permissions for .env file (readable/writable only by owner)
    chmod 600 .env
    echo ""

    # Display completion message
    echo "=========================================="
    echo "  Preparation Complete!"
    echo "=========================================="
    echo ""
    echo "Generated secure credentials:"
    echo "  COMPOSE_PROJECT_NAME:  ${INSTANCE_NAME}"
    echo "  SUB2API_IMAGE:         ${DEFAULT_IMAGE}"
    echo "  SERVER_PORT:           ${DEFAULT_PORT}"
    echo "  POSTGRES_PASSWORD:     ${POSTGRES_PASSWORD}"
    echo "  JWT_SECRET:            ${JWT_SECRET}"
    echo "  TOTP_ENCRYPTION_KEY:   ${TOTP_ENCRYPTION_KEY}"
    echo ""
    print_warning "These credentials have been saved to .env file."
    print_warning "Please keep them secure and do not share publicly!"
    echo ""
    echo "Directory structure:"
    echo "  docker-compose.yml        - Docker Compose configuration"
    echo "  .env                      - Environment variables (generated secrets)"
    echo "  .env.example              - Example template (for reference)"
    echo "  data/                     - Application data (will be created on first run)"
    echo "  postgres_data/            - PostgreSQL data"
    echo "  redis_data/               - Redis data"
    echo ""
    echo "Next steps:"
    echo "  1. (Optional) Edit .env to customize configuration"
    echo "  2. Start services:"
    echo "     docker compose up -d"
    echo ""
    echo "  3. View logs:"
    echo "     docker compose logs -f sub2api"
    echo "     tail -f data/logs/sub2api.log"
    echo ""
    echo "  4. Access Web UI:"
    echo "     http://localhost:${DEFAULT_PORT}"
    echo ""
    print_info "If admin password is not set in .env, it will be auto-generated."
    print_info "Check logs for the generated admin password on first startup."
    echo ""
}

# Run main function
main "$@"
