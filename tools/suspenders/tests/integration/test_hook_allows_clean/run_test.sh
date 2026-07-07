#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
TEST_CASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
IMAGE_NAME="suspenders-integration-test"

echo "Building Docker image ${IMAGE_NAME}..."
docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

echo "=== TEST: hook allows commit with clean file ==="

VOLUME_NAME="suspenders-test-hook-allows-clean-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo with the hook installed and a clean file staged
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        git init /home/testuser/testrepo
        suspenders hook install /home/testuser/testrepo
        cd /home/testuser/testrepo
        echo 'hello world' > clean.txt
        git add clean.txt
    "

# Exercise: run git commit — should succeed
OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cd /home/testuser/testrepo
        git commit -m 'add clean file'
    " 2>&1)
echo "${OUTPUT}"

# Assert: commit was created
COMMIT_COUNT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cd /home/testuser/testrepo
        git log --oneline | wc -l | tr -d ' '
    " 2>&1)
echo "Commit count: ${COMMIT_COUNT}"

if [ "${COMMIT_COUNT}" -lt 1 ]; then
    echo "ERROR: expected at least 1 commit, got ${COMMIT_COUNT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_allows_clean ==="
