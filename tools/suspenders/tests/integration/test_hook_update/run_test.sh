#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="suspenders-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

echo "=== TEST: hook update rewrites an outdated hook ==="

VOLUME_NAME="suspenders-test-hook-update-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo and install the hook
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        git init /home/testuser/testrepo
        suspenders hook install /home/testuser/testrepo
    "

# Tamper: replace the version checksum with a bogus value so the hook looks outdated
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        HOOK=/home/testuser/testrepo/.git/hooks/pre-commit
        sed -i 's/# version: .*/# version: outdated/' \"\${HOOK}\"
    "

# Assert: status reports outdated before update
STATUS_BEFORE=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook status /home/testuser/testrepo
    " 2>&1)
echo "Status before update: ${STATUS_BEFORE}"

if ! echo "${STATUS_BEFORE}" | grep -qF 'outdated'; then
    echo "ERROR: expected status 'outdated' before update, got: ${STATUS_BEFORE}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Exercise: run hook update
docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook update /home/testuser/testrepo
    "

# Assert: status reports installed after update
STATUS_AFTER=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook status /home/testuser/testrepo
    " 2>&1)
echo "Status after update: ${STATUS_AFTER}"

if ! echo "${STATUS_AFTER}" | grep -qF 'installed'; then
    echo "ERROR: expected status 'installed' after update, got: ${STATUS_AFTER}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_update ==="
