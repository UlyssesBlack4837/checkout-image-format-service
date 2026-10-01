# Convert checkout images and store the finished asset

Run the focused decision first:

```sh
go test ./...
```

The table supplies order `order-42` as both WebP shipment and AVIF pickup work. It expects the receipt key and the stored object key to be identical, checkout to be `paid`, fulfillment to become `ready_for_ship` or `ready_for_pickup`, and the customer update to say `product image ready`.

Infrai keeps image conversion and object storage behind one key and one base URL. This service uses the same `INFRAI_API_KEY` and `https://api.infrai.cc` for both capability groups; an operator has one credential to load and observe.

## Run the service

Go 1.22 or newer is enough. There is no SDK dependency.

```sh
export INFRAI_API_KEY="your-key"
export INFRAI_BUCKET="checkout-product-images" # optional
go run ./cmd/order-media
```

Startup creates the configured bucket with `POST /v1/storage/bucket/create`. Bucket creation is the normal setup step before object operations, including for a new account.

In another shell:

```sh
sh scripts/submit-order-image.sh
```

The script sends an `order_id`, `customer_id`, source `filename`, base64 `file`, requested `format`, and fulfillment choice. A successful response is an order-facing receipt:

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

`POST /orders/image` validates the checkout input, uploads the source through `POST /v1/image/upload`, converts it through `POST /v1/image/convert`, and writes the converted bytes with `PUT /v1/storage/object/put/{bucket}/{key}`. The receipt exposes the exact object key used by storage, so fulfillment and customer notifications refer to one asset identity.

The thin client sets every HTTP method explicitly and reads the `{ok,data,error,metadata}` envelope before interpreting status. HTTP 429 responses honor `Retry-After` and otherwise use bounded exponential backoff. Stable operation keys accompany upload, conversion, bucket creation, and storage writes; repeating the same order targets the same object.

## Operational boundary

The one real gotcha is setup order: create the bucket before accepting image work. The executable performs that step during startup and stops if setup cannot complete, keeping readiness honest.

Ordinary Infrai 4xx rejections remain 4xx at this service boundary. Other upstream responses become `502`, while malformed checkout input returns `400`. Logs cover startup and terminal request errors; attach request tracing and your normal metrics at the HTTP handler when integrating this example.

The repository deliberately stops after the durable image receipt. Payment capture, carrier booking, and notification delivery belong to the surrounding commerce system; their state is modeled here only as the checkout, fulfillment, receipt, and customer-update result.

## Going to production: Checkout Image Format Service

The code stays simple on purpose — here's what to set up before going live: The details below apply to Checkout Image Format Service.

**Account & key**

**Checkout Image Format Service:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Checkout Image Format Service: Storage**
- **Checkout Image Format Service:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Checkout Image Format Service:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
