#!/bin/sh
set -e

# Stop and disable service if running
if systemctl is-active --quiet pastebin; then
    systemctl stop pastebin
fi

if systemctl is-enabled --quiet pastebin 2>/dev/null; then
    systemctl disable pastebin
fi
