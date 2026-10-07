---
max_turns: 12
allowed_tools: [Read, Glob, Grep, Skill]
---
You are preparing a present page from the work summary below. The present MCP is not available in this environment, so do not try to call it. Instead, write the arguments you would pass to `present_create` as one JSON object with the keys `title`, `content`, and `graph`, inside a single ```json fenced block. Put the fenced block last in your reply.

Work summary (a test fixture): the services of the orders platform and who calls whom, for a brief that shows a new engineer who depends on whom and where the traffic is heaviest.

- ingress-gateway (Envoy) receives all traffic and routes to orders-api and catalog-api.
- orders-api calls pricing, stock, and payments-adapter. It writes to orders-db (Postgres) and publishes order.placed to the events topic (Kafka).
- catalog-api reads catalog-db (Postgres) and the search index (OpenSearch).
- mailer consumes order.placed from the events topic and calls the mail provider, an external SaaS.
- payments-adapter calls the payment provider, an external service.
- Traffic: ingress to orders-api about 1200 requests per second, ingress to catalog-api about 3000, orders-api to pricing 1200, orders-api to stock 1200.
