#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-hook-install-all-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: create 3 git repos and a suspenders config that excludes repo-c
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/repos/org/repo-a
        mkdir -p /home/testuser/repos/org/repo-b
        mkdir -p /home/testuser/repos/org/repo-c

        cd /home/testuser/repos/org/repo-a
        git init
        git commit --allow-empty -m 'initial commit'

        cd /home/testuser/repos/org/repo-b
        git init
        git commit --allow-empty -m 'initial commit'

        cd /home/testuser/repos/org/repo-c
        git init
        git commit --allow-empty -m 'initial commit'

        mkdir -p /home/testuser/.config/suspenders
        printf 'dirs:\n  - /home/testuser/repos\nexclude:\n  - \"*/repo-c\"\n' \
            > /home/testuser/.config/suspenders/config.yaml
    "

# Exercise: install hooks in all discovered repos
INSTALL_OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        suspenders hook install --all
    " 2>&1)

# Assert: repo-a and repo-b have hooks installed
HOOK_STATUS=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        [ -f /home/testuser/repos/org/repo-a/.git/hooks/pre-commit ] && echo 'REPO_A_HAS_HOOK' || echo 'REPO_A_NO_HOOK'
        [ -f /home/testuser/repos/org/repo-b/.git/hooks/pre-commit ] && echo 'REPO_B_HAS_HOOK' || echo 'REPO_B_NO_HOOK'
        [ -f /home/testuser/repos/org/repo-c/.git/hooks/pre-commit ] && echo 'REPO_C_HAS_HOOK' || echo 'REPO_C_NO_HOOK'
    " 2>&1)

if ! echo "${HOOK_STATUS}" | grep -qF 'REPO_A_HAS_HOOK'; then
    echo "ERROR: repo-a should have a hook installed"
    echo "${HOOK_STATUS}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${HOOK_STATUS}" | grep -qF 'REPO_B_HAS_HOOK'; then
    echo "ERROR: repo-b should have a hook installed"
    echo "${HOOK_STATUS}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

if ! echo "${HOOK_STATUS}" | grep -qF 'REPO_C_NO_HOOK'; then
    echo "ERROR: repo-c should NOT have a hook (it is excluded by config)"
    echo "${HOOK_STATUS}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Assert: the installed hooks are managed by suspenders
HOOK_CONTENT_A=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cat /home/testuser/repos/org/repo-a/.git/hooks/pre-commit
    " 2>&1)

if ! echo "${HOOK_CONTENT_A}" | grep -qF 'managed by suspenders'; then
    echo "ERROR: repo-a hook does not contain 'managed by suspenders'"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_install_all ==="
