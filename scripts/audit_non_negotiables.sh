#!/usr/bin/env bash
# shellcheck shell=bash
# - - - - - - - - - - - - - - - - - - - - - - - -
##@Version           :  202608131924-git
# @@Author           :  Jason Hempstead
# @@Contact          :  git-admin@casjaysdev.pro
# @@License          :  WTFPL
# @@ReadME           :  {scriptname --help | README.md}
# @@Copyright        :  Copyright: (c) 2026 Jason Hempstead, Casjays Developments
# @@Created          :  Thursday, August 13, 2026 19:24 EDT
# @@File             :  audit_non_negotiables.sh
# @@Description      :  Audits the project against all NON-NEGOTIABLE requirements in AI.md
# @@Changelog        :  Bring script into CasjaysDev header and lint compliance
# @@TODO             :  none
# @@Other            :  none
# @@Resource         :  none
# @@Terminal App     :  yes
# @@sudo/root        :  no
# @@Template         :  shell/bash
# - - - - - - - - - - - - - - - - - - - - - - - -
# shellcheck disable=SC1001,SC1003,SC2001,SC2003,SC2016,SC2031,SC2090,SC2115,SC2120,SC2155,SC2199,SC2229,SC2317,SC2329
# - - - - - - - - - - - - - - - - - - - - - - - -
#
# NON-NEGOTIABLE Compliance Audit Script
# This script audits the project against ALL NON-NEGOTIABLE requirements in AI.md
# Following PART 0 rules: Container-only development, fix issues directly, no reports
#
# Usage: ./scripts/audit_non_negotiables.sh

set -uo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASS=0
FAIL=0
WARN=0

# Project root
PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$PROJECT_ROOT"

# Detect project variables
PROJECTNAME=$(git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)(\.git)?$|\1|' || basename "$PROJECT_ROOT")
PROJECTORG=$(git remote get-url origin 2>/dev/null | sed -E 's|.*[:/]([^/]+)/[^/]+(\.git)?$|\1|' || basename "$(dirname "$PROJECT_ROOT")")

echo "╔════════════════════════════════════════════════════════════════════════════╗"
echo "║  NON-NEGOTIABLE COMPLIANCE AUDIT                                           ║"
echo "║  Project: $PROJECTNAME (${PROJECTORG})                                     ║"
echo "╚════════════════════════════════════════════════════════════════════════════╝"
echo ""

# Helper functions
__check_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASS++))
}

__check_fail() {
    echo -e "${RED}✗${NC} $1"
    echo -e "  ${RED}└─ $2${NC}"
    ((FAIL++))
}

__check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    echo -e "  ${YELLOW}└─ $2${NC}"
    ((WARN++))
}

__check_section() {
    echo -e "\n${BLUE}══════════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════════════════════${NC}"
}

__file_exists() {
    [[ -f "$1" ]]
}

__dir_exists() {
    [[ -d "$1" ]]
}

__file_contains() {
    grep -q -- "$2" "$1" 2>/dev/null
}

# Start audit
__check_section "PART 0: CRITICAL RULES - Licensing & Features"

# Licensing
if __file_exists "LICENSE.md"; then
    if __file_contains "LICENSE.md" "MIT License"; then
        __check_pass "MIT License present in LICENSE.md"
    else
        __check_fail "LICENSE.md exists but MIT License not found" "Must use MIT License for project code"
    fi
else
    __check_fail "LICENSE.md missing" "Required file - must contain MIT License"
fi

# No feature gating (check for actual gating patterns, not just words)
if grep -rE -- "(if|require|check).*\b(premium|pro|enterprise)\b|(upgrade|paid).*feature|license.*key.*check|tier.*(free|paid|premium)" src/ 2>/dev/null | grep -v -- "ssl-provider\|progress" || false; then
    __check_fail "Feature gating detected in code" "All features must be free and available to all users"
else
    __check_pass "No feature gating detected in source code"
fi

__check_section "PART 0: Build & Binary Rules"

# CGO_ENABLED=0
if __file_exists "Makefile"; then
    if grep -q -- "CGO_ENABLED=0" Makefile; then
        __check_pass "CGO_ENABLED=0 in Makefile"
    else
        __check_fail "CGO_ENABLED=0 not found in Makefile" "Must always build with CGO_ENABLED=0"
    fi
else
    __check_fail "Makefile missing" "Required for container-based builds"
fi

# Required platforms
REQUIRED_PLATFORMS=("linux-amd64" "linux-arm64" "darwin-amd64" "darwin-arm64" "windows-amd64" "windows-arm64" "freebsd-amd64" "freebsd-arm64")
if __file_exists "Makefile"; then
    # Check if using PLATFORMS variable or individual targets
    if grep -q -- "PLATFORMS.*:=.*linux.*darwin.*windows.*freebsd" Makefile 2>/dev/null; then
        # Check all 8 platforms are in PLATFORMS variable
        all_found=true
        for platform in "${REQUIRED_PLATFORMS[@]}"; do
            os="${platform%-*}"
            arch="${platform#*-}"
            if ! grep -q -- "$os/$arch" Makefile 2>/dev/null; then
                all_found=false
                __check_fail "Platform $platform missing" "All 8 platforms required (4 OS × 2 arch)"
            fi
        done
        if $all_found; then
            for platform in "${REQUIRED_PLATFORMS[@]}"; do
                __check_pass "Platform $platform in build targets"
            done
        fi
    else
        # Check individual platform targets
        for platform in "${REQUIRED_PLATFORMS[@]}"; do
            if grep -q -- "$platform" Makefile; then
                __check_pass "Platform $platform in build targets"
            else
                __check_fail "Platform $platform missing" "All 8 platforms required (4 OS × 2 arch)"
            fi
        done
    fi
fi

# Source directory
if __dir_exists "src"; then
    __check_pass "src/ directory exists"
else
    __check_fail "src/ directory missing" "All Go code must be in src/ directory"
fi

__check_section "PART 0: Container-Only Development"

# Makefile targets
REQUIRED_TARGETS=("dev" "local" "build" "test")
if __file_exists "Makefile"; then
    for target in "${REQUIRED_TARGETS[@]}"; do
        if grep -q -- "^${target}:" Makefile; then
            __check_pass "Makefile target: $target"
        else
            __check_fail "Makefile target missing: $target" "Required for container-based development"
        fi
    done
fi

__check_section "PART 0: Runtime Detection Rules"

# Check for hardcoded machine values (ignore fallbacks and file paths)
if grep -rE -- "hostname\s*:?=\s*\"[a-zA-Z0-9-]+\.(com|net|org|local)\"" src/ 2>/dev/null | grep -v -- "//"; then
    __check_fail "Hardcoded machine-dependent values found" "All machine settings must be detected at runtime"
else
    __check_pass "No hardcoded machine-dependent values detected"
fi

# Check for runtime detection
if grep -rq -- "os.Hostname()\|runtime.NumCPU()" src/ 2>/dev/null; then
    __check_pass "Runtime detection functions found"
else
    __check_warn "Runtime detection not found" "Should use os.Hostname(), runtime.NumCPU() for machine detection"
fi

__check_section "PART 0: JSON File Rules"

# Check for JSON comments
if grep -r -- "//\|/\*" **/*.json 2>/dev/null | grep -v -- ".git"; then
    __check_fail "Comments found in JSON files" "JSON does not support comments"
else
    __check_pass "No comments in JSON files"
fi

__check_section "PART 0: Docker Rules"

# Docker directory
if __dir_exists "docker"; then
    __check_pass "docker/ directory exists"
    
    if __file_exists "docker/Dockerfile"; then
        __check_pass "docker/Dockerfile exists"
        
        # Check for multi-stage
        if __file_contains "docker/Dockerfile" "FROM.*AS builder"; then
            __check_pass "Multi-stage Dockerfile"
        else
            __check_fail "Dockerfile not multi-stage" "Must use builder + runtime stages"
        fi
        
        # Check for required packages
        for pkg in tini curl bash git tor; do
            if __file_contains "docker/Dockerfile" "$pkg"; then
                __check_pass "Dockerfile includes $pkg"
            else
                __check_fail "Dockerfile missing $pkg" "Required package in runtime stage"
            fi
        done
        
        # Check STOPSIGNAL
        if __file_contains "docker/Dockerfile" "STOPSIGNAL.*SIGRTMIN"; then
            __check_pass "STOPSIGNAL SIGRTMIN+3"
        else
            __check_fail "STOPSIGNAL SIGRTMIN+3 not set" "Required for proper signal handling"
        fi
    else
        __check_fail "docker/Dockerfile missing" "Required for container deployment"
    fi
    
    if __file_exists "docker/docker-compose.yml"; then
        __check_pass "docker-compose.yml exists"
    else
        __check_fail "docker-compose.yml missing" "Required for production deployment"
    fi
else
    __check_fail "docker/ directory missing" "Required directory at project root"
fi

# Dockerfile in root (forbidden)
if __file_exists "Dockerfile"; then
    __check_fail "Dockerfile in project root" "Must be in docker/Dockerfile"
fi

__check_section "PART 0: CLI Rules"

# Check for required CLI flags
REQUIRED_FLAGS=("--help" "--version" "--mode" "--config" "--data" "--log" "--pid" "--address" "--port" "--debug" "--status" "--service" "--daemon" "--maintenance" "--update")

if __file_exists "src/main.go" || __file_exists "src/cli/cli.go"; then
    for flag in "${REQUIRED_FLAGS[@]}"; do
        flag_name="${flag#--}"  # Remove -- prefix
        if grep -rq -- "\"$flag_name\"" src/ 2>/dev/null; then
            __check_pass "CLI flag: $flag"
        else
            __check_fail "CLI flag missing: $flag" "Required NON-NEGOTIABLE flag"
        fi
    done
else
    __check_fail "src/main.go not found" "Cannot verify CLI flags"
fi

__check_section "PART 0: Directory Structure"

# Required directories
REQUIRED_DIRS=("src" "docker" "docs" "tests" "scripts")
for dir in "${REQUIRED_DIRS[@]}"; do
    if __dir_exists "$dir"; then
        __check_pass "Required directory: $dir/"
    else
        __check_fail "Required directory missing: $dir/" "Must exist per specification"
    fi
done

# Forbidden directories in root
FORBIDDEN_DIRS=("config" "data" "logs" "tmp" "temp" "build" "dist" "vendor" "node_modules" "cmd" "internal" "pkg")
for dir in "${FORBIDDEN_DIRS[@]}"; do
    if __dir_exists "$dir"; then
        __check_fail "Forbidden directory exists: $dir/" "Must not exist per specification"
    else
        __check_pass "Forbidden directory absent: $dir/"
    fi
done

__check_section "PART 0: File Naming Conventions"

# Check for forbidden files
FORBIDDEN_FILES=("SUMMARY.md" "COMPLIANCE.md" "NOTES.md" "CHANGELOG.md" "AUDIT.md" "REPORT.md" "ANALYSIS.md" "server.yml" "cli.yml" ".env")
for file in "${FORBIDDEN_FILES[@]}"; do
    if __file_exists "$file"; then
        __check_fail "Forbidden file exists: $file" "Must not exist per specification"
    else
        __check_pass "Forbidden file absent: $file"
    fi
done

# Check for required files
REQUIRED_FILES=("AI.md" "README.md" "LICENSE.md" "Makefile" "go.mod" "release.txt" ".gitignore" "mkdocs.yml")
for file in "${REQUIRED_FILES[@]}"; do
    if __file_exists "$file"; then
        __check_pass "Required file: $file"
    else
        __check_fail "Required file missing: $file" "Must exist per specification"
    fi
done

__check_section "PART 0: Boolean Handling"

# Check for strconv.ParseBool (forbidden)
if grep -rq -- "strconv.ParseBool" src/ 2>/dev/null; then
    __check_fail "strconv.ParseBool found in code" "Must use config.ParseBool() for all boolean parsing"
else
    __check_pass "No strconv.ParseBool usage (correct: use config.ParseBool)"
fi

# Check for config.ParseBool
if __file_exists "src/config/bool.go"; then
    __check_pass "src/config/bool.go exists"
    if __file_contains "src/config/bool.go" "ParseBool"; then
        __check_pass "config.ParseBool() function defined"
    else
        __check_warn "config.ParseBool() not found in bool.go" "Should implement comprehensive boolean parsing"
    fi
else
    __check_fail "src/config/bool.go missing" "Required for proper boolean handling"
fi

__check_section "PART 0: Database Rules"

# Check for bcrypt (forbidden)
if grep -rq -- "bcrypt" src/ 2>/dev/null | grep -v -- "NEVER.*bcrypt"; then
    __check_fail "bcrypt usage detected" "Must use Argon2id for password hashing"
else
    __check_pass "No bcrypt usage (correct: use Argon2id)"
fi

# Check for Argon2id
if grep -rq -- "argon2" src/ 2>/dev/null; then
    __check_pass "Argon2 usage detected (correct)"
else
    __check_warn "Argon2 not found" "Should use Argon2id for password hashing"
fi

__check_section "PART 0: What NOT To Do"

# Check for .env files
if ls .env* 2>/dev/null | grep -v -- ".gitignore" >/dev/null; then
    __check_fail ".env files found" "Must not use .env files - hardcode defaults in docker-compose"
else
    __check_pass "No .env files (correct)"
fi

# Check for Dockerfile in root
if __file_exists "Dockerfile"; then
    __check_fail "Dockerfile in root" "Must be in docker/Dockerfile"
fi

# Check for docker-compose.yml in root
if __file_exists "docker-compose.yml"; then
    __check_fail "docker-compose.yml in root" "Must be in docker/docker-compose.yml"
fi

__check_section "PART 8: CLI Flags"

# Short flags (only -h and -v allowed)
if grep -rE -- "flag\.(String|Int|Bool).*\"-[a-gi-u]" src/ 2>/dev/null; then
    __check_fail "Unauthorized short flags found" "Only -h (help) and -v (version) allowed"
else
    __check_pass "No unauthorized short flags"
fi

__check_section "PART 26: Makefile Requirements"

if __file_exists "Makefile"; then
    # Check for GODIR and GOCACHE
    if __file_contains "Makefile" "GODIR\|GOCACHE"; then
        __check_pass "Makefile uses GODIR/GOCACHE for caching"
    else
        __check_warn "GODIR/GOCACHE not found" "Should use local caching for faster rebuilds"
    fi
    
    # Check for Docker usage in targets
    if __file_contains "Makefile" "docker run.*golang\|GO_DOCKER"; then
        __check_pass "Makefile uses Docker for builds"
    else
        __check_fail "Makefile doesn't use Docker" "Must use container-based builds"
    fi
fi

__check_section "PART 27: Docker Compose Files"

COMPOSE_FILES=("docker/docker-compose.yml" "docker/docker-compose.dev.yml" "docker/docker-compose.test.yml")
for compose in "${COMPOSE_FILES[@]}"; do
    if __file_exists "$compose"; then
        __check_pass "Compose file exists: $compose"
    else
        __check_warn "Compose file missing: $compose" "Recommended for different environments"
    fi
done

__check_section "PART 28: CI/CD Workflows"

# Check for workflow files
if __dir_exists ".github/workflows" || __dir_exists ".gitea/workflows"; then
    __check_pass "CI/CD workflow directory exists"
    
    WORKFLOW_FILES=("release.yml" "beta.yml" "daily.yml" "docker.yml")
    for wf in "${WORKFLOW_FILES[@]}"; do
        if __file_exists ".github/workflows/$wf" || __file_exists ".gitea/workflows/$wf"; then
            __check_pass "Workflow exists: $wf"
        else
            __check_warn "Workflow missing: $wf" "Recommended for automated releases"
        fi
    done
else
    __check_warn "No CI/CD workflows found" "Should have .github/workflows or .gitea/workflows"
fi

__check_section "PART 29: Testing & Development"

# Check for test scripts
TEST_SCRIPTS=("tests/run_tests.sh" "tests/docker.sh" "tests/incus.sh")
for script in "${TEST_SCRIPTS[@]}"; do
    if __file_exists "$script"; then
        __check_pass "Test script exists: $script"
        # Check if executable
        if [[ -x "$script" ]]; then
            __check_pass "Test script is executable: $script"
        else
            __check_fail "Test script not executable: $script" "chmod +x $script"
        fi
    else
        __check_warn "Test script missing: $script" "Recommended for automated testing"
    fi
done

__check_section "PART 30: ReadTheDocs Documentation"

# MkDocs configuration
if __file_exists "mkdocs.yml"; then
    __check_pass "mkdocs.yml exists"
    
    if __file_contains "mkdocs.yml" "material"; then
        __check_pass "MkDocs Material theme configured"
    else
        __check_warn "MkDocs Material theme not configured" "Should use Material theme"
    fi
else
    __check_fail "mkdocs.yml missing" "Required for ReadTheDocs"
fi

if __file_exists ".readthedocs.yaml"; then
    __check_pass ".readthedocs.yaml exists"
else
    __check_fail ".readthedocs.yaml missing" "Required for ReadTheDocs"
fi

# Required documentation pages
DOC_PAGES=("docs/index.md" "docs/installation.md" "docs/configuration.md" "docs/api.md" "docs/admin.md" "docs/development.md")
for page in "${DOC_PAGES[@]}"; do
    if __file_exists "$page"; then
        __check_pass "Documentation page exists: $page"
    else
        __check_fail "Documentation page missing: $page" "Required for complete docs"
    fi
done

__check_section "PART 36: CLI Client"

# Check for CLI client
if __dir_exists "src/client"; then
    __check_pass "src/client/ directory exists (CLI client)"
    
    if __file_exists "src/client/main.go"; then
        __check_pass "src/client/main.go exists"
    else
        __check_fail "src/client/main.go missing" "Required for CLI client"
    fi
else
    __check_warn "src/client/ not found" "CLI client is required for all projects"
fi

__check_section "PART 36: Agent (if applicable)"

# Check for agent (optional)
if __dir_exists "src/agent"; then
    __check_pass "src/agent/ directory exists (optional)"
    
    if __file_exists "src/agent/main.go"; then
        __check_pass "src/agent/main.go exists"
    else
        __check_fail "src/agent/main.go missing" "If src/agent/ exists, must have main.go"
    fi
else
    __check_pass "src/agent/ not present (optional component)"
fi

__check_section "PART 37: Project-Specific Requirements"

# Check if PART 37 is filled in AI.md
if __file_exists "AI.md"; then
    if grep -A 10 -- "^# PART 37:" AI.md | grep -q -- "IDEA.md\|Data:\|Endpoints:\|Business Rules:"; then
        __check_pass "PART 37 appears to be filled in AI.md"
    else
        __check_fail "PART 37 not filled in AI.md" "Must document project-specific requirements"
    fi
else
    __check_fail "AI.md missing" "Required specification file"
fi

# Check for IDEA.md (recommended)
if __file_exists "IDEA.md"; then
    __check_pass "IDEA.md exists (project vision)"
else
    __check_warn "IDEA.md not found" "Recommended for separating WHAT from HOW"
fi

# Summary
echo ""
echo "╔════════════════════════════════════════════════════════════════════════════╗"
echo "║  AUDIT SUMMARY                                                             ║"
echo "╚════════════════════════════════════════════════════════════════════════════╝"
echo -e "${GREEN}✓ PASS:${NC} $PASS"
echo -e "${YELLOW}⚠ WARN:${NC} $WARN"
echo -e "${RED}✗ FAIL:${NC} $FAIL"
echo ""

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}✓ All NON-NEGOTIABLE requirements met!${NC}"
    exit 0
else
    echo -e "${RED}✗ $FAIL NON-NEGOTIABLE requirement(s) failed${NC}"
    echo -e "${YELLOW}Fix all failures and re-run audit${NC}"
    exit 1
fi

# ex: ts=2 sw=2 et filetype=sh
