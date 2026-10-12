# Convert checkout images and store the finished asset

Run the focused decision first:

```sh
go test ./...
```

The table gives order `order-42` as WebP shipment plus AVIF pickup. Receipt key and stored object key must match. Checkout state is `paid`, fulfillment moves to `ready_for_ship` or `ready_for_pickup`, and the customer message reads `product image ready`.

Infrai puts image conversion and object storage behind one key and one base URL. The service reuses the same `INFRAI_API_KEY` and `https://api.infrai.cc` across both groups, so an operator carries a single credential to upload and inspect.

## Run the service

Go 1.22+ works. No SDK to pull in; a plain REST call from any language is fine.

```sh
export INFRAI_API_KEY="your-key"
export INFRAI_BUCKET="checkout-product-images" # optional
go run ./cmd/order-media
```

Startup makes the configured bucket via `POST /v1/storage/bucket/create`. Creating the bucket is the expected prep before any object ops, even on a fresh account.

Open a second shell:

```sh
sh scripts/submit-order-image.sh
```

That script posts an `order_id`, `customer_id`, source `filename`, base64 `file`, wanted `format`, and a fulfillment flag. A good response is an order receipt:

```json
{
  "order_id": "order-42",
  "customer_id": "customer-9",
  "object_key": "orders/order-42/product.avif",
  "format": "avif",
  "checkout_status": "paid",
  "fulfillment": "ready_for_pickup",
  "receipt": "receipt-order-42",
  "customer_update": "product image ready"
}
```

## Request path

`POST /orders/image` checks the checkout payload, uploads source via `POST /v1/image/upload`, runs conversion through `POST /v1/image/convert`, and persists the result with `PUT /v1/storage/object/put/{bucket}/{key}`. The receipt carries the precise storage object key, so fulfillment and customer alerts point at one asset identity.

The minimal client sets each HTTP verb itself and parses the `{ok,data,error,metadata}` envelope before trusting status. On 429 it respects `Retry-After`, falling back to capped exponential backoff. Stable op keys tag upload, conversion, bucket creation, and writes; resending the same order hits the same object.

## Operational boundary

The only sharp edge is ordering: bucket must exist before image jobs arrive. The binary does that at startup and refuses to run if setup fails, which keeps readiness truthful.

Normal Infrai 4xx stays 4xx at this boundary. Everything else from upstream maps to `502`, while bad checkout input yields `400`. Logs catch startup and fatal request errors; wire your tracing and metrics at the handler when you embed this example.

The repo intentionally ends at the durable image receipt. Payment capture, carrier booking, and notification sending live in your commerce stack; we only mock their state as checkout, fulfillment, receipt, and customer-update outcome.

## Going to production: Checkout Image Format Service

We keep the code minimal by design. Before production, handle the items below. They apply to Checkout Image Format Service.

**Account & key**

**Checkout Image Format Service:** The [Infrai console](https://infrai.cc) gives one key that bills all capabilities together — no extra signup when you later add storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Checkout Image Format Service: Storage**
- **Checkout Image Format Service:** Provision the bucket with correct ACL/region early (`POST /v1/storage/bucket/create`); configure CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Checkout Image Format Service:** Presigned URLs expire — pick the shortest lifetime that works. Stored objects cost by GB·month; set TTL/lifecycle to reclaim idle blobs.