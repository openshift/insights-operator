#!/bin/sh

# Please keep in mind that when the comments mention a "source archive",
# they are referring to a directory containing an _extracted_ IO archive.

if [ -z "$1" ]; then
    >&2 echo "Usage: update_sample_archive.sh <Extracted Archive Source Directory> [JSON Content Filter]"
    exit
fi

# This allows the JSON-finding function to read the filter from
# a global variable instead of having to pass it as an argument.
CONTENT_FILTER="$2"

# Get absolute path of the source IO archive.
SOURCE_PREFIX=$(realpath "$1")/

# Get absolute path of the IO sample archive directory.
SAMPLE_PREFIX=$(realpath "$(dirname "$0")/../docs/insights-archive-sample")/

# Escape dots and brackets (the most likely special characters found in paths)
# with backslashes to prevent breaking the regular expressions.
regexEscape() {
    echo "$1" | sed 's/[][)(}{\.]/\\\0/g'
}

# Escaped version of the directory paths ready to be used in regular expressions.
SOURCE_PREFIX_ESCAPED=$(regexEscape "$SOURCE_PREFIX")
SAMPLE_PREFIX_ESCAPED=$(regexEscape "$SAMPLE_PREFIX")

jq_update_file() {
    source_file="$SOURCE_PREFIX$1"
    if [ ! -f "$source_file" ]; then
        >&2 echo "[WARN] Unable to update file '$1' (file not found in the source archive)"
        return 1
    fi

    sample_file="$SAMPLE_PREFIX$1"
    mkdir -p "${sample_file%/*}"
    jq < "$source_file" > "$sample_file" || exit 1
    echo "[OK] $source_file --> $sample_file"
}

jq_update_dir() {
    source_dir="$SOURCE_PREFIX$1"
    if [ ! -d "$source_dir" ]; then
        >&2 echo "[WARN] Unable to update directory '$1' (directory not found in the source archive)"
        return 1
    fi

    sample_dir="$SAMPLE_PREFIX$1"
    # Delete the old JSON files.
    [ -d "$sample_dir" ] && find "$sample_dir" -name '*.json' -type f -delete
    # Copy and format JSON files from the source archive to the sample archive directory.
    find "$SOURCE_PREFIX$1" -name '*.json' | grep -oP "^${SOURCE_PREFIX_ESCAPED}\K.+" | sort | uniq | while read -r fname; do
        jq_update_file "$fname"
    done
}

# Expression used when looking for unique directories containing found files.
FIND_DIR_EXPR='/(?=[^/:]+'
# Expression used when looking for all found files.
FIND_FILE_EXPR='(?='

# If a content filter was provided, then all JSON files that match the filter in the existing sample archive directory are returned.
# Otherwise, a complete list of JSON files in the existing sample archive directory structure is returned.
# The first argument is used switch between returning a list of files and a list of unique directories containing said files.
find_jsons() {
    if [ -z "$CONTENT_FILTER" ]; then
        # find "$SOURCE_PREFIX" -iname "*.json" | grep -oP "^${SOURCE_PREFIX_ESCAPED}\K[^:]+${1})" | sort | uniq
        find "$SAMPLE_PREFIX" -iname "*.json" | grep -oP "^${SAMPLE_PREFIX_ESCAPED}\K[^:]+${1})" | sort | uniq
    else
        grep -rn "$SAMPLE_PREFIX" --include \*.json -e "$CONTENT_FILTER" | grep -oP "^${SAMPLE_PREFIX_ESCAPED}\K[^:]+?${1}:)" | sort | uniq
    fi
}

# Return value indicating if the specified directory is known to contain files with randomized names.
# This function only checks the path prefix, which means that subdirectory/file paths can be checked as well.
contains_randomized_names() {
    case "$1" in
        config/certificatesigningrequests/*|\
        config/hostsubnet/*|\
        config/machineconfigs/*|\
        config/node/*|\
        config/persistentvolumes/*|\
        config/pod/*|\
        machinesets/*)
            true
            ;;

        *)
            false
            ;;
    esac
}

# If one of the resources in a directory contains a filter hit, the whole directory must be updated
# because some resource names are randomized and repeated sample archive updates would result in
# size inflation of the sample archive (i.e., more and more pod resource JSONs with each archive update).
# There is a list of directories which contain files with randomized names.
# Remaning directories are handled on a file-by-file basis.
find_jsons "$FIND_DIR_EXPR" | while read -r dir_name; do
    contains_randomized_names "$dir_name" && jq_update_dir "$dir_name"
done

# This handles the remaining files after the entire directories of resources have already been updated.
find_jsons "$FIND_FILE_EXPR" | while read -r file_name; do
    contains_randomized_names "$file_name" || jq_update_file "$file_name"
done
