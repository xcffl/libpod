#!/bin/bash

set -eo pipefail

source $(dirname $0)/lib.sh

req_env_var CI UPLDREL_IMAGE CIRRUS_BUILD_ID GOSRC RELEASE_GCPJSON RELEASE_GCPNAME RELEASE_GCPROJECT

[[ "$CI" == "true" ]] || \
    die 56 "$0 must be run under Cirrus-CI to function"

SWAGGER_FILEPATH="pkg/api/swagger.yaml"

# We store "releases" for each PR, mostly to validate the process is functional
unset PR_OR_BRANCH BUCKET
if [[ -n "$CIRRUS_PR" ]]
then
    PR_OR_BRANCH="pr$CIRRUS_PR"
    BUCKET="libpod-pr-releases"
elif [[ -n "$CIRRUS_BRANCH" ]]
then
    # Only release binaries for docs
    if [[ $CIRRUS_TASK_NAME =~ "docs" ]]
    then
        PR_OR_BRANCH="$CIRRUS_BRANCH"
        BUCKET="libpod-$CIRRUS_BRANCH-releases"
    else
        warn "" "Skipping release processing for non-docs task."
        exit 0
    fi
else
    die 1 "Expecting either \$CIRRUS_PR or \$CIRRUS_BRANCH to be non-empty."
fi

# Functional local podman required for uploading
echo "Verifying a local, functional podman, building one if necessary."
[[ -n "$(type -P podman)" ]] || \
    make install PREFIX=/usr || \
    die 57 "$0 requires working podman binary on path to function"

TMPF=$(mktemp -p '' $(basename $0)_XXXX.json)
trap "rm -f $TMPF" EXIT
set +x
echo "$RELEASE_GCPJSON" > "$TMPF"
[[ "$OS_RELEASE_ID" == "ubuntu" ]] || \
    chcon -t container_file_t "$TMPF"
unset RELEASE_GCPJSON

cd $GOSRC
for filename in $(ls -1 $SWAGGER_FILEPATH)
do
    unset EXT
    EXT=$(echo "$filename" | sed -r -e 's/.+\.(.+$)/\1/g')
    if [[ -z "$EXT" ]] || [[ "$EXT" == "$filename" ]]
    then
        echo "Warning: Not processing $filename (invalid extension '$EXT')"
        continue
    fi
    if [[ "$EXT" =~ "gz" ]]
    then
        EXT="tar.gz"
    fi

    if [[ $filename == $SWAGGER_FILEPATH ]]
    then
        # Support other tools referencing branch and/or version-specific refs.
        TO_FILENAME="swagger-${RELEASE_VERSION}-${PR_OR_BRANCH}.yaml"
        # For doc. ref. this must always be a static filename, e.g. swagger-latest-master.yaml
        ALSO_FILENAME="swagger-latest-${PR_OR_BRANCH}.yaml"
    else
        die "Uploading non-docs files has been disabled"
    fi

    [[ "$OS_RELEASE_ID" == "ubuntu" ]] || \
        chcon -t container_file_t "$filename"

    echo "Running podman ... $UPLDREL_IMAGE for $filename -> $TO_FILENAME"
    podman run -i --rm \
        -e "GCPNAME=$RELEASE_GCPNAME" \
        -e "GCPPROJECT=$RELEASE_GCPROJECT" \
        -e "GCPJSON_FILEPATH=$TMPF" \
        -e "FROM_FILEPATH=/tmp/$filename" \
        -e "TO_FILENAME=$TO_FILENAME" \
        -e "ALSO_FILENAME=$ALSO_FILENAME" \
        -e "PR_OR_BRANCH=$PR_OR_BRANCH" \
        -e "BUCKET=$BUCKET" \
        -v "$TMPF:$TMPF:ro" \
        -v "$(realpath $GOSRC/$filename):/tmp/$filename:ro" \
        $UPLDREL_IMAGE
done
