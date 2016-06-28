#!/bin/bash

set -e -x

usage()
{
  echo "Usage: $0 mount <ssh url> <mountpoint>"
	echo "       $0 umount <mountpoint>"
}

if [[ "$1" != "mount" && "$1" != "umount" ]]; then
  usage
  exit 1
fi

if [[ "$1" == "mount" ]]; then
	mode="mount"
	shift
  if [[ "$#" != "2" ]]; then
  	usage
    exit 1
  fi
  ssh_url="$1"
  shift
fi

if [[ "$1" == "umount" ]]; then
	mode="umount"
	shift
  if [[ "$#" != "1" ]]; then
  	usage
    exit 1
  fi
fi

mount_point="$1"
shift

sshfs_mount_point="$(dirname "${mount_point}")_ssh/$(basename "${mount_point}")"
bindfs_mount_source="$(dirname "${sshfs_mount_point}")"
bindfs_mount_point="$(dirname "${mount_point}")"

mount_base_dir="$(echo "${mount_point}" | sed -e 's!^/var/lib/docker-volumes/_sshfs/!!' -e 's!/.*$!!')"

if [[ "${mode}" == "mount" ]]; then

  mkdir -p "${mount_point}"
  mkdir -p "${sshfs_mount_point}"

  sshfs -o IdentityFile=/root/.ssh/id_rsa -o uid=1000 -o gid=1000 -o default_permissions -o allow_other -o idmap=user -o follow_symlinks -o transform_symlinks -o sshfs_debug "${ssh_url}" "${sshfs_mount_point}"

  bindfs -o nonempty -u 1000 -g 1000 -o allow_other --create-for-user=1000 --create-for-group=1000 --chown-ignore --chgrp-ignore "${bindfs_mount_source}" "${bindfs_mount_point}"

  pushd /var/lib/docker-volumes/_sshfs
    find "${mount_base_dir}" -xdev -xtype d \! -user 1000 -print0 | xargs -0 -r chown 1000:1000
  popd

fi

if [[ "${mode}" == "umount" ]]; then
	umount "${bindfs_mount_point}"
	umount "${sshfs_mount_point}"

	pushd /var/lib/docker-volumes/_sshfs
  	rmdir --ignore-fail-on-non-empty -p "$(echo "${sshfs_mount_point}" | sed -e 's!^/var/lib/docker-volumes/_sshfs/!!')"
		rmdir --ignore-fail-on-non-empty -p "$(echo "${bindfs_mount_point}" | sed -e 's!^/var/lib/docker-volumes/_sshfs/!!')"
	popd
fi
