#!/bin/sh
set -eu

mkdir -p /run/sshd

if [ ! -f /home/bot/keys/ssh_host_ed25519_key ]; then
    echo "Generating SSH host key..."
    ssh-keygen -t ed25519 -f /home/bot/keys/ssh_host_ed25519_key -N ""
fi

exec /usr/sbin/sshd -D -p 2222 \
    -h /home/bot/keys/ssh_host_ed25519_key \
    -o PidFile=/tmp/sshd.pid \
    -o AuthorizedKeysFile=/home/bot/.ssh/authorized_keys \
    -o PasswordAuthentication=no \
    -o PubkeyAuthentication=yes \
    -o UsePAM=no \
    -o PermitRootLogin=no \
    -o AllowTcpForwarding=yes \
    -o X11Forwarding=no \
    -o PermitTunnel=no \
    -o GatewayPorts=no \
    -o MaxAuthTries=3 \
    -o LoginGraceTime=20
