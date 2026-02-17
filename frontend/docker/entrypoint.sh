#!/bin/sh
set -e

# Substitute environment variables in nginx config
envsubst '${DOCKER_CD_API_BASE_URL}' < /etc/nginx/conf.d/default.conf > /etc/nginx/conf.d/default.conf.tmp
mv /etc/nginx/conf.d/default.conf.tmp /etc/nginx/conf.d/default.conf

# Also inject API base URL into the JS bundle at runtime
# This replaces a placeholder in index.html
if [ -n "$DOCKER_CD_API_BASE_URL" ]; then
  # Create a runtime config script
  cat > /usr/share/nginx/html/config.js <<EOF
window.__DOCKER_CD_CONFIG__ = {
  apiBaseUrl: "${DOCKER_CD_API_BASE_URL}"
};
EOF
fi

exec "$@"
