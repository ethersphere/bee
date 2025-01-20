#!/bin/bash

# Base URL
base_url="http:/localhost:1633/chunks"

# File containing paths
file_path="refs.txt"


success_count=0

# Read each line from the file
while IFS= read -r line
do
    # Construct the full URL
    url="${base_url}/${line}"
    
    # Fetch the URL
    echo "$url"
   
    # response=$(curl "$url" -s -o /dev/null -w "%{http_code}")

    # if [ "$response" -eq 200 ]; then
    #     success_count=$((success_count + 1))
    # fi

done < "$file_path"

# Output the count of successful requests
echo "Number of successful requests: $success_count"
