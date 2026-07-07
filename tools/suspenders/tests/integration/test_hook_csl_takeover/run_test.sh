#!/usr/bin/env bash
set -e

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../../.." && pwd)
IMAGE_NAME="suspenders-integration-test"

docker build -t ${IMAGE_NAME} ${PROJECT_ROOT} -f ${PROJECT_ROOT}/tools/suspenders/Dockerfile

VOLUME_NAME="suspenders-test-hook-csl-takeover-$(date +%s)"
docker volume create ${VOLUME_NAME} > /dev/null

# Setup: a git repo with a csl-managed post-merge hook, plus a suspenders config
# whose csl-reindex post_merge entry appends the repo path to the reindex queue.
# Both the csl hook and the suspenders entry would queue a reindex on merge; if
# the csl hook were chained, every merge would queue TWICE. Taking it over must
# leave exactly one append per merge.
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "
        set -e
        mkdir -p /home/testuser/testrepo
        cd /home/testuser/testrepo
        git init -q
        git commit -q --allow-empty -m 'initial commit'

        # csl's own post-merge hook: carries the csl marker and appends to the
        # queue itself. If suspenders chained to it, it would run on every merge.
        cat > /home/testuser/testrepo/.git/hooks/post-merge <<'EOF'
#!/bin/sh
# csl-managed-hook: reindex on merge
mkdir -p \"\${HOME}/.config/csl\"
printf '%s\n' \"\$(git rev-parse --show-toplevel)\" >> \"\${HOME}/.config/csl/reindex.queue\"
EOF
        chmod +x /home/testuser/testrepo/.git/hooks/post-merge

        # suspenders config: the recommended csl-reindex post_merge entry.
        mkdir -p /home/testuser/.config/suspenders
        cat > /home/testuser/.config/suspenders/config.yaml <<'EOF'
dirs:
  - /home/testuser
hooks:
  post_merge:
    - name: csl-reindex
      command: |
        mkdir -p \"\${HOME}/.config/csl\" &&
        printf '%s\n' \"\$(git rev-parse --show-toplevel)\" >> \"\${HOME}/.config/csl/reindex.queue\"
EOF

        # Take over the hooks.
        suspenders hook install /home/testuser/testrepo
    "

# Assert: the csl hook must NOT be left as an executable post-merge.backup
# (the always-chain script runs <event>.backup; a csl .backup would double-queue).
BACKUP_CHECK=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        if [ -e /home/testuser/testrepo/.git/hooks/post-merge.backup ]; then
            echo 'BACKUP_PRESENT'
        else
            echo 'BACKUP_ABSENT'
        fi
        if [ -f /home/testuser/testrepo/.git/hooks/post-merge.csl-replaced ]; then
            echo 'CSL_REPLACED_PRESENT'
        fi
    " 2>&1)

if echo "${BACKUP_CHECK}" | grep -qF 'BACKUP_PRESENT'; then
    echo "ERROR: csl takeover left an executable post-merge.backup (would double-queue)"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi
if ! echo "${BACKUP_CHECK}" | grep -qF 'CSL_REPLACED_PRESENT'; then
    echo "ERROR: csl hook was not parked at post-merge.csl-replaced"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Exercise: perform a real merge so git fires the post-merge hook once.
QUEUE_COUNT=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        set -e
        cd /home/testuser/testrepo
        git checkout -q -b feature
        printf 'feature work\n' > feature.txt
        git add feature.txt
        git commit -q -m 'feature commit'
        git checkout -q master 2>/dev/null || git checkout -q main
        git merge -q --no-ff feature -m 'merge feature'
        wc -l < /home/testuser/.config/csl/reindex.queue | tr -d ' '
    " 2>&1)

if [ "${QUEUE_COUNT}" != "1" ]; then
    echo "ERROR: expected exactly 1 reindex append per merge, got: '${QUEUE_COUNT}'"
    docker run --rm -v "${VOLUME_NAME}:/home/testuser" --entrypoint /bin/sh \
        ${IMAGE_NAME} -c "cat /home/testuser/.config/csl/reindex.queue" 2>&1 || true
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Assert: re-running install stays stable — still no resurrected .backup.
docker run --rm --entrypoint /bin/sh \
    -v "${VOLUME_NAME}:/home/testuser" \
    ${IMAGE_NAME} -c "suspenders hook install /home/testuser/testrepo" > /dev/null 2>&1

REINSTALL_CHECK=$(docker run --rm \
    -v "${VOLUME_NAME}:/home/testuser" \
    --entrypoint /bin/sh \
    ${IMAGE_NAME} -c "
        if [ -e /home/testuser/testrepo/.git/hooks/post-merge.backup ]; then
            echo 'BACKUP_PRESENT'
        else
            echo 'BACKUP_ABSENT'
        fi
    " 2>&1)

if echo "${REINSTALL_CHECK}" | grep -qF 'BACKUP_PRESENT'; then
    echo "ERROR: re-running install resurrected an executable post-merge.backup"
    docker volume rm ${VOLUME_NAME} > /dev/null
    exit 1
fi

# Cleanup
docker volume rm ${VOLUME_NAME} > /dev/null
echo "=== TEST PASSED: test_hook_csl_takeover ==="
