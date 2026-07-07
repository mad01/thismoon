#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="suspenders-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

echo "=== TEST: hook install writes pre-commit hook ==="

VOLUME_NAME="suspenders-test-hook-install-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo in the volume
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        git init /home/testuser/testrepo
    "

# Exercise: install the hook
OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook install /home/testuser/testrepo
    " 2>&1)
echo "${OUTPUT}"

# Assert: pre-commit file exists
VERIFY=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        HOOK=/home/testuser/testrepo/.git/hooks/pre-commit

        if [ ! -f \"\${HOOK}\" ]; then
            echo 'ERROR: pre-commit hook file does not exist'
            exit 1
        fi
        echo 'pre-commit file exists: OK'

        if ! grep -qF 'managed by suspenders' \"\${HOOK}\"; then
            echo 'ERROR: hook does not contain managed by suspenders marker'
            exit 1
        fi
        echo 'managed by suspenders marker: OK'

        if [ ! -x \"\${HOOK}\" ]; then
            echo 'ERROR: pre-commit hook is not executable'
            exit 1
        fi
        echo 'hook is executable: OK'
    " 2>&1)
echo "${VERIFY}"

if echo "${VERIFY}" | grep -q 'ERROR:'; then
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_install ==="
