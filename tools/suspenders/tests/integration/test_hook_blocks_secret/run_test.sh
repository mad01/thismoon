#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-hook-blocks-secret-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo with a file containing an AWS access key
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo
        cd /home/testuser/testrepo
        git init
        git commit --allow-empty -m 'initial commit'
        suspenders hook install /home/testuser/testrepo
        printf 'AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n' > secrets.env
        git add secrets.env
    "

# Exercise: attempt git commit — it should fail because of the secret
OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cd /home/testuser/testrepo
        if git commit -m 'add secrets'; then
            echo 'COMMIT_SUCCEEDED'
        else
            echo 'COMMIT_BLOCKED'
        fi
    " 2>&1)

# Assert: commit must have been blocked
if echo "${OUTPUT}" | grep -qF 'COMMIT_SUCCEEDED'; then
    echo "ERROR: commit should have been blocked by the pre-commit hook"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${OUTPUT}" | grep -qF 'COMMIT_BLOCKED'; then
    echo "ERROR: expected COMMIT_BLOCKED in output, got:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${OUTPUT}" | grep -qF 'potential secret'; then
    echo "ERROR: expected 'potential secret' in hook output, got:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_blocks_secret ==="
