#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-history-clean-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: a repo with a secret buried in history.
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo
        cd /home/testuser/testrepo
        git init -b main
        printf 'clean file\n' > readme.txt
        git add readme.txt
        git commit --no-verify -m 'initial commit'
        printf 'token=SUPERSECRETVALUE9000\n' > config.txt
        git add config.txt
        git commit --no-verify -m 'add config with SUPERSECRETVALUE9000'
        git rm -q config.txt
        git commit --no-verify -m 'remove config'
    "

# Exercise: rewrite history, replacing the secret.
OUTPUT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        cd /home/testuser/testrepo
        suspenders history clean --yes \
            --replace SUPERSECRETVALUE9000
        echo '--- verify ---'
        if git log --all -p --format='%H %ae %B' | grep -qF 'SUPERSECRETVALUE9000'; then
            echo 'SECRET_STILL_PRESENT'
        else
            echo 'SECRET_GONE'
        fi
        if git log --all -p | grep -qF '***REDACTED***'; then
            echo 'PLACEHOLDER_PRESENT'
        fi
        if [ \"\$(git show HEAD:readme.txt)\" = 'clean file' ]; then
            echo 'TREE_INTACT'
        fi
        ls \$(git rev-parse --absolute-git-dir)/suspenders-backup-*.bundle > /dev/null && echo 'BACKUP_EXISTS'
    " 2>&1)

for expected in SECRET_GONE PLACEHOLDER_PRESENT TREE_INTACT BACKUP_EXISTS; do
    if ! echo "${OUTPUT}" | grep -qF "${expected}"; then
        echo "ERROR: expected ${expected} in output, got:"
        echo "${OUTPUT}"
        docker volume rm ${VOLUME_NAME} > /dev/null
        exit 1
    fi
done

if echo "${OUTPUT}" | grep -qF 'SECRET_STILL_PRESENT'; then
    echo "ERROR: secret survived the history clean:"
    echo "${OUTPUT}"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_history_clean ==="
