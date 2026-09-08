# Multi-stage build for the SPA: node builds the Vite bundle, nginx serves it
# and reverse-proxies /api to the Gateway on the compose network (same shape as
# the Vite dev proxy, so the app stays same-origin).
# syntax=docker/dockerfile:1

# ---- build ----
FROM node:20-alpine@sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build   # tsc --noEmit && vite build -> dist/

# ---- runtime ----
# nginx-unprivileged: runs as the non-root `nginx` user and defaults to
# listening on 8080, matching nginx.conf (no root master process needed).
FROM nginxinc/nginx-unprivileged:1.27-alpine@sha256:65e3e85dbaed8ba248841d9d58a899b6197106c23cb0ff1a132b7bfe0547e4c0
# envsubst template: AAKS_RESOLVER / AAKS_GATEWAY_UPSTREAM are rendered at
# container startup (see nginx.conf); everything else in the config is static.
ENV AAKS_RESOLVER="kube-dns.kube-system.svc.cluster.local" \
    AAKS_GATEWAY_UPSTREAM="gateway.aaks.svc.cluster.local:8080"
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
USER nginx
