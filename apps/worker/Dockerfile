FROM node:22-bookworm-slim
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev --no-audit --no-fund
COPY server.mjs ./
ENV NODE_ENV=production PORT=8090
EXPOSE 8090
USER node
CMD ["node", "server.mjs"]
