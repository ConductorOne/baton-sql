#!/bin/bash

# This is an example script. DO NOT add actual credentials here.
# Instead, set them in your environment before running this script.

# For security, export these credentials in your terminal, not in this file
# export BATON_CLIENT_ID="your-client-id"
# export BATON_CLIENT_SECRET="your-client-secret" 
# export BATON_C1_API_HOST="your-api-host"

# Verify credentials are set
if [ -z "$BATON_CLIENT_ID" ] || [ -z "$BATON_CLIENT_SECRET" ] || [ -z "$BATON_C1_API_HOST" ]; then
  echo "Error: One or more required environment variables are not set."
  echo "Please set the following environment variables before running this script:"
  echo "  BATON_CLIENT_ID"
  echo "  BATON_CLIENT_SECRET"
  echo "  BATON_C1_API_HOST"
  exit 1
fi

# Build the connector if needed
if [ ! -f "dist/darwin_arm64/baton-sql" ]; then
  echo "Building baton-sql..."
  make build
fi

# Run the connector in service mode
./dist/darwin_arm64/baton-sql \
  --config-path ./examples/mysql-production.yml \
  --log-level debug