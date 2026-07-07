#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-history-scan-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: a repo where a secret was committed in the past and removed again.
# The working tree is clean, so only a history scan can find it.
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo
        cd /home/testuser/testrepo
        git init -b main
        git commit --allow-empty -m 'initial commit'
        printf 'AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n' > secrets.env
        git add secrets.env
        git commit --no-verify -m 'add secret by mistake'
        git rm -q secrets.env
        git commit --no-verify -m 'remove secret'
    "

# Exercise: history scan must flag the introducing commit and exit nonzero.
OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cd /home/testuser/testrepo
        if suspenders history scan --fail-on-findings; then
            echo 'SCAN_PASSED'
        else
            echo 'SCAN_FLAGGED'
        fi
    " 2>&1)

if ! echo "${OUTPUT}" | grep -qF 'SCAN_FLAGGED'; then
    echo "ERROR: history scan should have flagged the removed secret, got:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${OUTPUT}" | grep -qF 'add secret by mistake'; then
    echo "ERROR: expected the introducing commit subject in output, got:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${OUTPUT}" | grep -qF 'aws-access-key-id'; then
    echo "ERROR: expected the aws-access-key-id rule in output, got:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_history_scan ==="
