#!/usr/bin/env bash
set -eu

input_dir="input"
output_dir="output"

shopt -s nocaseglob
for img in "$input_dir"/*.{jpg,jpeg}; do
    echo "Processing $img"
    output_file="$output_dir/$(basename "$img")"
    magick "$img" \
        -auto-orient \
        -background black \
        -gravity center \
        -extent "%[fx:w/h > (16/9) ? w : h*(16/9)]x%[fx:w/h > (16/9) ? w/(16/9) : h]" \
        "$output_file"
done
