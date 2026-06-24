#!/usr/bin/env bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

get_latest_tag() {
    git fetch --tags --quiet 2>/dev/null || true
    git tag -l 'v*' | sort -V | tail -1
}

bump_version() {
    local current="$1"
    local type="$2"

    local major minor patch
    IFS='.' read -r major minor patch <<< "$current"

    case "$type" in
        major) echo "$((major + 1)).0.0" ;;
        minor) echo "${major}.$((minor + 1)).0" ;;
        patch) echo "${major}.${minor}.$((patch + 1))" ;;
    esac
}

CURRENT_TAG=$(get_latest_tag)

if [ -z "$CURRENT_TAG" ]; then
    CURRENT_VERSION="0.1.0"
else
    CURRENT_VERSION="${CURRENT_TAG#v}"
fi

if [ $# -eq 0 ]; then
    echo "Usage: $0 [patch|minor|major|<exact-version>]"
    echo ""
    echo "Current version: ${GREEN}v${CURRENT_VERSION}${NC}"
    echo ""
    echo "Examples:"
    echo "  $0 patch    → bump patch: v${CURRENT_VERSION} → v$(bump_version "$CURRENT_VERSION" patch)"
    echo "  $0 minor    → bump minor: v${CURRENT_VERSION} → v$(bump_version "$CURRENT_VERSION" minor)"
    echo "  $0 major    → bump major: v${CURRENT_VERSION} → v$(bump_version "$CURRENT_VERSION" major)"
    echo "  $0 1.5.0    → set exact:  v${CURRENT_VERSION} → v1.5.0"
    exit 1
fi

case "$1" in
    patch|minor|major)
        NEW_VERSION=$(bump_version "$CURRENT_VERSION" "$1")
        ;;
    *)
        NEW_VERSION="$1"
        if ! echo "$NEW_VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
            echo -e "${RED}Error: version must be in format X.Y.Z (e.g., 1.5.0)${NC}"
            exit 1
        fi
        ;;
esac

NEW_TAG="v${NEW_VERSION}"

echo -e "Current:  ${YELLOW}${CURRENT_TAG:-none}${NC}"
echo -e "New tag:  ${GREEN}${NEW_TAG}${NC}"
echo ""

if git rev-parse "$NEW_TAG" >/dev/null 2>&1; then
    echo -e "${RED}Error: tag ${NEW_TAG} already exists${NC}"
    exit 1
fi

echo -n "Proceed? [y/N] "
read -r confirm
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
    echo "Cancelled."
    exit 0
fi

git tag "$NEW_TAG"
git push origin "$NEW_TAG"

echo ""
echo -e "${GREEN}Release ${NEW_TAG} pushed!${NC}"
echo ""
echo "CI will now:"
echo "  1. Build binary dockify-linux-amd64 v${NEW_VERSION}"
echo "  2. Create GitHub Release with binary attached"
echo "  3. Push Docker image ghcr.io/coderbuzz/dockify:${NEW_TAG}"
echo ""
echo "Watch: https://github.com/coderbuzz/dockify/actions"
