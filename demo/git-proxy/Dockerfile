FROM nginx:1.31-alpine
RUN apk add --no-cache openssl
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY entrypoint.sh /demo-entrypoint.sh
ENTRYPOINT ["/bin/sh", "/demo-entrypoint.sh"]
