#!/bin/sh
set -e

# Create pastebin user if it doesn't exist
if ! getent passwd pastebin > /dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin pastebin
fi

# Reload systemd
systemctl daemon-reload

echo "Pastebin installed successfully."
echo "Configure /etc/default/pastebin and then run:"
echo "  systemctl enable pastebin"
echo "  systemctl start pastebin"
