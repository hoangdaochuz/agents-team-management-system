# Multi-stage build for the SPA: node builds the Vite bundle, nginx serves it
# and reverse-proxies /api to the Gateway on the compose network (same shape as
# the Vite dev proxy, so the app stays same-origin).
# syntax=docker/dockerfile:1

# ---- build ----
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build   # tsc --noEmit && vite build -> dist/

# ---- runtime ----
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
