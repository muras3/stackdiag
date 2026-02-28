FROM nginx:alpine

# Install bash and openssl for cert generation
RUN apk add --no-cache bash openssl libfaketime

# Copy cert generation script and generate certs at build time
COPY certs/generate.sh /tmp/generate.sh
RUN mkdir -p /etc/nginx/certs && \
    cd /etc/nginx/certs && \
    bash /tmp/generate.sh && \
    rm /tmp/generate.sh

# Remove default config
RUN rm /etc/nginx/conf.d/default.conf
