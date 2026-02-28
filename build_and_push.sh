#!/bin/bash
set -e

# Configuration
DOCKER_USERNAME="lisp19"
IMAGE_NAME="localrouter"
REPO="${DOCKER_USERNAME}/${IMAGE_NAME}"

# Require jq and curl mapping tools
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed."
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo "Error: curl is required but not installed."
    exit 1
fi

echo "Fetching latest tags from Docker Hub for $REPO..."

# Fetch the list of tags from Docker Hub using unauthenticated API
# Sort and filter for semantic versioning tags (e.g., v1.0.0)
LATEST_TAG=$(curl -s "https://hub.docker.com/v2/repositories/${REPO}/tags/?page_size=100" | jq -r '.results[].name' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1)

if [ -z "$LATEST_TAG" ]; then
    echo "No valid vX.Y.Z tags found on regular Docker Hub. Starting from v1.0.0."
    NEXT_TAG="v1.0.0"
else
    echo "Found latest tag: $LATEST_TAG"
    
    # Extract version components
    # Remove 'v' prefix
    VERSION=${LATEST_TAG#v}
    
    # Split into major, minor, patch
    IFS='.' read -r major minor patch <<< "$VERSION"
    
    # Simple semantic version bump rule for automated scripts: bump patch by default
    patch=$((patch + 1))
    
    NEXT_TAG="v${major}.${minor}.${patch}"
    echo "Incremented version automatically to: $NEXT_TAG"
fi

echo "==============================="
echo "Building $REPO:$NEXT_TAG"
echo "==============================="

# Build the new image and tag it with semantic version and latest
docker build -t "${REPO}:${NEXT_TAG}" -t "${REPO}:latest" .

echo "==============================="
echo "Pushing $REPO:$NEXT_TAG and latest"
echo "==============================="

docker push "${REPO}:${NEXT_TAG}"
docker push "${REPO}:latest"

echo "Success! Image published as ${REPO}:${NEXT_TAG} and ${REPO}:latest"
