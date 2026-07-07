#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-hook-chain-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo with a pre-existing pre-commit hook, then install suspenders
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo/.git/hooks
        cd /home/testuser/testrepo
        git init
        git commit --allow-empty -m 'initial commit'

        # Write the original pre-commit hook
        printf '#!/bin/sh\necho original-hook-ran > /home/testuser/testrepo/hook_marker\n' \
            > /home/testuser/testrepo/.git/hooks/pre-commit
        chmod +x /home/testuser/testrepo/.git/hooks/pre-commit

        # Install suspenders — should chain the existing hook
        suspenders hook install /home/testuser/testrepo
    "

# Assert: pre-commit.backup must exist
BACKUP_CHECK=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        if [ -f /home/testuser/testrepo/.git/hooks/pre-commit.backup ]; then
            echo 'BACKUP_EXISTS'
        else
            echo 'BACKUP_MISSING'
        fi
    " 2>&1)

if ! echo "${BACKUP_CHECK}" | grep -qF 'BACKUP_EXISTS'; then
    echo "ERROR: pre-commit.backup was not created"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Assert: pre-commit hook contains "managed by suspenders" and references the backup
HOOK_CONTENT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cat /home/testuser/testrepo/.git/hooks/pre-commit
    " 2>&1)

if ! echo "${HOOK_CONTENT}" | grep -qF 'managed by suspenders'; then
    echo "ERROR: pre-commit hook does not contain 'managed by suspenders'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${HOOK_CONTENT}" | grep -qF 'pre-commit.backup'; then
    echo "ERROR: pre-commit hook does not reference the backup hook"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Exercise: commit a clean file — should succeed and original hook should run
COMMIT_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        set -e
        cd /home/testuser/testrepo
        printf 'hello world\n' > clean.txt
        git add clean.txt
        git commit -m 'add clean file'
        echo 'COMMIT_SUCCEEDED'
    " 2>&1)

if ! echo "${COMMIT_OUTPUT}" | grep -qF 'COMMIT_SUCCEEDED'; then
    echo "ERROR: commit of clean file should have succeeded, got:"
    echo "${COMMIT_OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Assert: hook_marker must exist and contain "original-hook-ran"
MARKER_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        if [ -f /home/testuser/testrepo/hook_marker ]; then
            cat /home/testuser/testrepo/hook_marker
        else
            echo 'MARKER_MISSING'
        fi
    " 2>&1)

if ! echo "${MARKER_OUTPUT}" | grep -qF 'original-hook-ran'; then
    echo "ERROR: original hook did not run — hook_marker missing or wrong content: ${MARKER_OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_chain ==="
