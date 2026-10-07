---
type: llm
weight: 2
---
The reply ends with one JSON object for present_create with `title`, `content`, and `graph`. You see only the reply, so the fixture it was written from is repeated here.

Fixture: ingress-gateway (Envoy) routes to orders-api and catalog-api. orders-api calls pricing, stock, and payments-adapter, writes to orders-db (Postgres), and publishes order.placed to the events topic (Kafka). catalog-api reads catalog-db (Postgres) and the search index (OpenSearch). mailer consumes order.placed from the events topic and calls the mail provider (external SaaS). payments-adapter calls the payment provider (external). Traffic: ingress to orders-api 1200 requests per second, ingress to catalog-api 3000, orders-api to pricing 1200, orders-api to stock 1200.

Required, every one must hold:

1. The dependency map is the page's `graph` argument: `nodes` with `id` and `label`, `edges` with `from` and `to`, and a `layout` that is `dagre`, `elk`, or one of the `elk-` engines (or no layout, which means dagre). It is not drawn as a `diagram` block and not written only as a list of "A calls B".
2. The edges match the fixture: ingress-gateway to orders-api and to catalog-api; orders-api to pricing, stock, payments-adapter, orders-db, and the events topic; catalog-api to catalog-db and to the search index; mailer connected to the events topic; mailer to the mail provider; payments-adapter to the payment provider. No service or link the fixture does not name.
3. The four traffic figures (3000, 1200, 1200, 1200) are `weight` values on the matching edges. Edges the fixture gives no figure for carry no weight.
4. `content` places exactly one `{"t": "graph"}` block, not inside a `details` block.

Secondary, fail only when two or more of these miss:

5. Node colour is consistent: every node has a `tone`, or none has, or exactly one node is toned as the point of the map, or nodes use `type: "module"` with `color` slots. Not slots and tones together. Tone names (`neutral`, `green`, `red`, `blue`, `amber`, `purple`) are correct usage, not colour literals.
6. The weighted edges set `flow: true`, so the traffic animates.
7. A paragraph beside or above the graph block says what to look at (the heaviest edges, the external services).
8. The publish and consume links on the events topic use `type: "publishes"` or a `label`, so a reader can tell them from direct calls.

Fail outright if the map is a diagram block, if the reply tried to call an MCP tool instead of writing the JSON, or if there is no JSON object.
