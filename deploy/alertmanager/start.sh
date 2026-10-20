#!/bin/sh
set -eu
umask 077
# Alertmanager does not expand environment variables in its YAML configuration.
printf '%s' "${ALERT_WEBHOOK_URL:-http://127.0.0.1:65535/replace-me}" > /run/alertmanager/webhook_url
if [ "${1:-}" = "--check" ]; then
  exec /bin/amtool check-config /etc/alertmanager/alertmanager.yml
fi
exec /bin/alertmanager \
  --config.file=/etc/alertmanager/alertmanager.yml \
  --storage.path=/alertmanager \
  --web.listen-address=:9093 \
  --cluster.listen-address=
