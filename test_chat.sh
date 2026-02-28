#!/bin/bash

# Configuration
DEFAULT_BASE_URL="http://localhost:8080/v1"
BASE_URL="${1:-$DEFAULT_BASE_URL}"
ENDPOINT="${BASE_URL}/chat/completions"

echo "================================================="
echo "Testing LocalRouter Chat Completions API"
echo "Endpoint: $ENDPOINT"
echo "================================================="

# Create a temporary file to store the JSON payload
PAYLOAD_FILE=$(mktemp)

cat > "$PAYLOAD_FILE" << EOF
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "Hello! Please reply with a short greeting and tell me what model you are."
    }
  ],
  "temperature": 0.7,
  "stream": false
}
EOF

echo "Sending non-streaming request..."
curl -s -X POST "$ENDPOINT" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer sk-test-dummy-key" \
     -d @"$PAYLOAD_FILE" | jq .

echo -e "\n\n================================================="
echo "Sending streaming request (SSE)..."
echo "================================================="

cat > "$PAYLOAD_FILE" << EOF
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {
      "role": "user",
      "content": "Count from 1 to 5 slowly."
    }
  ],
  "temperature": 0.7,
  "stream": true
}
EOF

# Use curl to stream the response, preserving newlines
curl -N -s -X POST "$ENDPOINT" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer sk-test-dummy-key" \
     -d @"$PAYLOAD_FILE"

# Clean up
rm -f "$PAYLOAD_FILE"

echo -e "\n\n================================================="
echo "Test completed."
echo "If connection was refused, check if LocalRouter is running on $BASE_URL"
echo "Usage: ./test_chat.sh [http://custom-url:port/v1]"
