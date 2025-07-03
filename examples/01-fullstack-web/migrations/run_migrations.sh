#!/bin/bash

# Simple migration runner for the example
# In a real app, you'd use a proper migration tool

echo "[db] Checking for pending migrations..."

# Create a migrations tracking file if it doesn't exist
MIGRATIONS_FILE=".migrations_applied"
touch $MIGRATIONS_FILE

# Process each SQL file in order
for migration in migrations/*.sql; do
    filename=$(basename "$migration")
    
    # Check if migration was already applied
    if grep -q "$filename" "$MIGRATIONS_FILE"; then
        echo "[db] Migration $filename already applied"
    else
        echo "[db] Applying migration: $filename"
        # In a real app, you'd run this against your database
        # For now, we just simulate it
        echo "$filename" >> "$MIGRATIONS_FILE"
        echo "[db] Migration $filename applied successfully"
    fi
done

echo "[db] All migrations processed"