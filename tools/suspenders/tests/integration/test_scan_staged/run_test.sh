#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-scan-staged-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create a git repo with a staged Stripe key
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo
        cd /home/testuser/testrepo
        git init
        git commit --allow-empty -m 'initial commit'
        printf 'STRIPE_KEY=sk_live_1234567890abcdefghijklmn\n' > secrets.env
        git add secrets.env
    "

# Exercise part 1: scan staged files — should detect the Stripe key
SCAN_EXIT=0
SCAN_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        set -e
        suspenders scan --staged --fail-on-findings /home/testuser/testrepo
    " 2>&1) || SCAN_EXIT=$?

# Assert: exit non-zero (fail-on-findings)
if [ "${SCAN_EXIT}" -eq 0 ]; then
    echo "ERROR: scan should have exited non-zero due to Stripe key finding"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${SCAN_OUTPUT}" | grep -qiF 'stripe'; then
    echo "ERROR: expected 'stripe' in scan output, got:"
    echo "${SCAN_OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${SCAN_OUTPUT}" | grep -qF 'potential secret'; then
    echo "ERROR: expected 'potential secret' in scan output, got:"
    echo "${SCAN_OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Setup part 2: unstage the Stripe file, stage only a clean file
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        cd /home/testuser/testrepo
        git reset HEAD secrets.env
        printf 'hello world\n' > clean.txt
        git add clean.txt
    "

# Exercise part 2: scan staged files — should find nothing (Stripe file not staged)
CLEAN_SCAN_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders scan --staged /home/testuser/testrepo
    " 2>&1)

# Assert: no findings for clean staged file
if ! echo "${CLEAN_SCAN_OUTPUT}" | grep -qF 'No findings'; then
    echo "ERROR: expected 'No findings' when only clean file is staged, got:"
    echo "${CLEAN_SCAN_OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_scan_staged ==="
