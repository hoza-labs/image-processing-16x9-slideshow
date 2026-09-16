#!/usr/bin/env bash
set -eu

input_dir="16x9 a-v final - random names"

filenum=1
for file in "$input_dir"/*; do
    if [[ -f "$file" ]]; then
        extension="${file##*.}"
        while :; do
            random_name=$(tr -dc 'a-z0-9' </dev/urandom | head -c 5)
            new_file="$input_dir/$random_name.$extension"
            [[ -e "$new_file" ]] || break
        done

        mv -- "$file" "$new_file"
        echo "$filenum: Renamed $file to $new_file"
        ((filenum++))
    fi
done
