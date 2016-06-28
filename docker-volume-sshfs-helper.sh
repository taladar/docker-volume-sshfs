#!/bin/bash

set -e -x

sshfs -o IdentityFile=/root/.ssh/id_rsa -o uid=1000 -o gid=1000 -o allow_other -o idmap=user -o follow_symlinks -o transform_symlinks -o sshfs_debug "$1" "$2"

