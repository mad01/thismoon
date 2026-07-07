#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="suspenders-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

echo "=== TEST: hook uninstall restores original hook ==="

VOLUME_NAME="suspenders-test-hook-uninstall-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo with an existing pre-commit hook, then install suspenders
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        git init /home/testuser/testrepo
        mkdir -p /home/testuser/testrepo/.git/hooks
        printf '#!/bin/sh\necho original hook\n' > /home/testuser/testrepo/.git/hooks/pre-commit
        chmod +x /home/testuser/testrepo/.git/hooks/pre-commit
        suspenders hook install /home/testuser/testrepo
    "

# Assert: backup exists after install
BACKUP_CHECK=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        if [ ! -f /home/testuser/testrepo/.git/hooks/pre-commit.backup ]; then
            echo 'ERROR: pre-commit.backup does not exist after install'
            exit 1
        fi
        echo 'pre-commit.backup exists: OK'
    " 2>&1)
echo "${BACKUP_CHECK}"

if echo "${BACKUP_CHECK}" | grep -q 'ERROR:'; then
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Exercise: uninstall the hook
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook uninstall /home/testuser/testrepo
    "

# Assert: original hook is restored and backup is removed
VERIFY=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        HOOK=/home/testuser/testrepo/.git/hooks/pre-commit
        BACKUP=/home/testuser/testrepo/.git/hooks/pre-commit.backup

        if ! grep -qF 'original hook' \"\${HOOK}\"; then
            echo 'ERROR: pre-commit does not contain original hook content'
            exit 1
        fi
        echo 'original hook restored: OK'

        if [ -f \"\${BACKUP}\" ]; then
            echo 'ERROR: pre-commit.backup still exists after uninstall'
            exit 1
        fi
        echo 'pre-commit.backup removed: OK'
    " 2>&1)
echo "${VERIFY}"

if echo "${VERIFY}" | grep -q 'ERROR:'; then
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_uninstall ==="
