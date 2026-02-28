#!/bin/bash
set -e

# Default settings
DEFAULT_CONFIG_DIR="$HOME/.config/localrouter"
IMAGE_NAME="lisp19/localrouter:latest"
PORT="8080"

# Parse args
CONFIG_DIR="${1:-$DEFAULT_CONFIG_DIR}"

echo "Starting LocalRouter via Docker..."
echo "Configuration directory mapped to: $CONFIG_DIR"
echo "Listening on port: $PORT"

# Ensure config directory exists locally
mkdir -p "$CONFIG_DIR"

# Check if docker is installed
if ! command -v docker &> /dev/null; then
    echo "Error: docker is not installed. Please install Docker first."
    exit 1
fi

# Pull the latest image
echo "Pulling latest image: $IMAGE_NAME"
docker pull "$IMAGE_NAME"

# Run the container in detached mode
# -d: Run container in background and print container ID
# -p 8080:8080: Map port 8080
# -v ...: Map local config directory to /app/config in container
# --name: Assign a name to the container
# --restart unless-stopped: Always restart the container if it stops, unless explicitly stopped

# Stop and remove existing container if it exists
if [ "$(docker ps -aq -f name=localrouter)" ]; then
    echo "Removing existing localrouter container..."
    docker rm -f localrouter
fi

echo "Starting new container..."
docker run -d \
    --name localrouter \
    -p "$PORT:8080" \
    -v "$CONFIG_DIR:/app/config" \
    --restart unless-stopped \
    "$IMAGE_NAME"

echo "LocalRouter is now running!"
echo "You can check logs with: docker logs -f localrouter"
echo "API endpoint: http://localhost:$PORT/v1/chat/completions"
