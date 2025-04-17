#!/bin/bash
# Script to validate employee_id and last_login features in baton-sql

# Function to display status messages
display() {
  echo -e "\n\033[1;36m$1\033[0m"
}

# Function to check if a file contains text
check_contains() {
  if grep -q "$2" "$1"; then
    echo -e "\033[32m✓ Found: $2\033[0m"
    return 0
  else
    echo -e "\033[31m✗ Not found: $2\033[0m"
    return 1
  fi
}

# Set the C1Z file path
C1Z_FILE="sync.c1z"

# Ensure the C1Z file exists
if [ ! -f "$C1Z_FILE" ]; then
  echo "Error: The sync.c1z file does not exist. Please run the connector first."
  exit 1
fi

display "Validating Employee ID Implementation"
# Extract the C1Z file
TMP_DIR=$(mktemp -d)
unzip -q -o "$C1Z_FILE" -d "$TMP_DIR"

# Check for employee_id in the resources
display "Checking for Employee ID in user resources"
USER_FILES=$(find "$TMP_DIR" -type f -name "*.json" | grep -i "resources/user")

SUCCESS_COUNT=0
TOTAL_COUNT=0

for file in $USER_FILES; do
  ((TOTAL_COUNT++))
  display "Checking file: $(basename "$file")"
  if check_contains "$file" "employee_id"; then
    ((SUCCESS_COUNT++))
  fi
done

display "Checking for Last Login in user resources"
LAST_LOGIN_SUCCESS=0

for file in $USER_FILES; do
  display "Checking file: $(basename "$file")"
  if check_contains "$file" "last_login"; then
    ((LAST_LOGIN_SUCCESS++))
  fi
done

# Clean up
rm -rf "$TMP_DIR"

# Display summary
display "Validation Summary"
echo "Employee ID: $SUCCESS_COUNT/$TOTAL_COUNT resources contain employee ID"
echo "Last Login: $LAST_LOGIN_SUCCESS/$TOTAL_COUNT resources contain last login"

if [ "$SUCCESS_COUNT" -gt 0 ] && [ "$LAST_LOGIN_SUCCESS" -gt 0 ]; then
  display "✅ VALIDATION SUCCESSFUL: Both features are working"
  exit 0
else
  display "❌ VALIDATION FAILED: One or more features are not working correctly"
  exit 1
fi